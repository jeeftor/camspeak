package api

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	neturl "net/url"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	clog "github.com/charmbracelet/log"
	"github.com/labstack/echo/v4"

	"github.com/jeeftor/camspeak/internal/cameras"
)

// streamSession tracks a live ffmpeg → camera stream so it can be stopped or
// paused on demand. Pause suspends the ffmpeg process via SIGSTOP without
// tearing down the camera speaker connection; resume sends SIGCONT.
// levelBits holds the current audio level (0.0–1.0) for VU meter display
// as a uint64 (via math.Float64bits), updated atomically by the
// levelTapReader on each Read from ffmpeg stdout.
type streamSession struct {
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	paused    bool
	url       string
	started   time.Time
	levelBits uint64
}

var (
	activeStreams   = make(map[string]*streamSession)
	activeStreamsMu sync.Mutex
)

// levelTapReader wraps an io.Reader and continuously samples the audio
// level for VU meter display. It reads from the underlying reader (ffmpeg
// stdout) and writes to the destination (cam.Stream) while computing a
// running RMS level from the raw µ-law bytes.
//
// The level is stored on the streamSession and updated atomically on each
// Read. The actual reading is passthrough — the tap only reads what the
// downstream consumer (cam.Stream) reads, so it doesn't buffer or block.
//
// levelTapReader forwards the Deadliner interface (SetReadDeadline) if the
// underlying reader implements it (e.g. *os.File from cmd.StdoutPipe),
// so CopyAt8kBps can still use read deadlines for responsive stopping.
type levelTapReader struct {
	r       io.Reader
	session *streamSession
}

func (t *levelTapReader) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	if n > 0 {
		level := computeLevel(p[:n])
		atomic.StoreUint64(&t.session.levelBits, math.Float64bits(level))
	}
	return n, err
}

// SetReadDeadline forwards to the underlying reader if it implements
// Deadliner (e.g. *os.File). This allows CopyAt8kBps to set read
// deadlines through the tap wrapper.
func (t *levelTapReader) SetReadDeadline(d time.Time) error {
	if dl, ok := t.r.(interface{ SetReadDeadline(time.Time) error }); ok {
		return dl.SetReadDeadline(d)
	}
	return nil
}

// computeLevel computes a normalized audio level (0.0–1.0) from a buffer
// of raw G.711 µ-law samples. µ-law bytes are 8-bit unsigned values
// centered at 128. We compute the RMS of the linear-decoded samples and
// normalize to [0, 1].
func computeLevel(buf []byte) float64 {
	if len(buf) == 0 {
		return 0
	}
	var sumSq float64
	for _, b := range buf {
		// Decode µ-law to linear PCM (16-bit range: -32124 to +32256).
		// The decode is a simplified version — we only need the magnitude.
		linear := mulawDecode(b)
		f := float64(linear)
		sumSq += f * f
	}
	rms := math.Sqrt(sumSq / float64(len(buf)))
	// Normalize: max linear value is ~32256. RMS of full-scale is ~32256.
	// Apply a log-ish curve so quiet audio is still visible.
	normalized := rms / 32256.0
	if normalized < 0 {
		normalized = 0
	}
	// Square root compression: makes quiet signals more visible.
	return math.Sqrt(normalized)
}

// mulawDecode decodes a single G.711 µ-law byte to a 16-bit linear sample.
func mulawDecode(b byte) int16 {
	b = ^b
	sign := (b & 0x80) >> 7
	segment := (b & 0x70) >> 4
	magnitude := b & 0x0F
	val := int16((magnitude << 3) + 0x84)
	val <<= segment
	if sign == 0 {
		val = -val
	}
	return val
}

// getStreamLevels returns the current audio level (0.0–1.0) for each
// camera that has an active stream. Safe for concurrent access.
func getStreamLevels() map[string]float64 {
	activeStreamsMu.Lock()
	defer activeStreamsMu.Unlock()
	out := make(map[string]float64, len(activeStreams))
	for cam, s := range activeStreams {
		if s.paused {
			out[cam] = 0
		} else {
			out[cam] = math.Float64frombits(atomic.LoadUint64(&s.levelBits))
		}
	}
	return out
}

// resolveStreamURL turns a playlist URL into the actual stream URL.
// Supports .pls and .m3u/.m3u8 playlists.
func resolveStreamURL(rawURL string) (string, error) {
	parsed, err := neturl.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid url: %w", err)
	}

	path := strings.ToLower(parsed.Path)
	switch {
	case strings.HasSuffix(path, ".pls"):
		return resolvePLS(rawURL)
	case strings.HasSuffix(path, ".m3u"), strings.HasSuffix(path, ".m3u8"):
		return resolveM3U(rawURL)
	default:
		return rawURL, nil
	}
}

// playlistClient is the HTTP client used for fetching .pls/.m3u playlists.
// It has a bounded timeout so a slow playlist host doesn't block the
// /api/play-stream or /api/play request indefinitely.
var playlistClient = &http.Client{Timeout: 10 * time.Second}

func resolvePLS(rawURL string) (string, error) {
	base, err := neturl.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid url: %w", err)
	}

	resp, err := playlistClient.Get(rawURL)
	if err != nil {
		return "", fmt.Errorf("fetching pls: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching pls: HTTP %d", resp.StatusCode)
	}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(strings.ToLower(line), "file1=") {
			val := strings.TrimSpace(line[len("File1="):])
			// Resolve relative URLs against the playlist base URL.
			u, err := base.Parse(val)
			if err != nil {
				return val, nil
			}
			return u.String(), nil
		}
		// Case-insensitive fallback.
		if strings.Contains(strings.ToLower(line), "file1=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				val := strings.TrimSpace(parts[1])
				u, err := base.Parse(val)
				if err != nil {
					return val, nil
				}
				return u.String(), nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("reading pls: %w", err)
	}
	return "", fmt.Errorf("no File1 entry found in PLS")
}

func resolveM3U(rawURL string) (string, error) {
	base, err := neturl.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid url: %w", err)
	}

	resp, err := playlistClient.Get(rawURL)
	if err != nil {
		return "", fmt.Errorf("fetching m3u: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching m3u: HTTP %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		u, err := base.Parse(line)
		if err != nil {
			return line, nil
		}
		return u.String(), nil
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("reading m3u: %w", err)
	}
	return "", fmt.Errorf("no stream entries found in M3U")
}

// PlayStream handles POST /api/play-stream — live audio stream → camera.
func (h *Handlers) PlayStream(c echo.Context) error {
	log := h.logger(c)

	var req struct {
		Camera string  `json:"camera"`
		URL    string  `json:"url"`
		Gain   float64 `json:"gain"`
	}
	if err := c.Bind(&req); err != nil || req.Camera == "" || req.URL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "camera and url required")
	}

	parsedURL, err := neturl.Parse(req.URL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return echo.NewHTTPError(http.StatusBadRequest, "url must be http or https")
	}

	cam, err := h.reg.Get(req.Camera)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	streamURL, err := resolveStreamURL(req.URL)
	if err != nil {
		log.Warn("stream: failed to resolve playlist", "url", req.URL, "err", err)
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}

	if err := h.startStreamToCamera(log, cam, req.Camera, streamURL, req.URL, req.Gain); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "streaming"})
}

// buildStreamFFmpegCmd creates and starts an ffmpeg process that reads a
// live stream URL and outputs raw G.711 µ-law 8kHz mono to stdout.
// The audio filter chain applies loudnorm (EBU R128 normalization to -16 LUFS,
// matching uploaded presets) followed by the camera gain, with optional
// prime silence (adelay) to warm the camera's audio engine.
// Uses -loglevel info so ICY metadata is logged to stderr for parsing.
func buildStreamFFmpegCmd(
	ctx context.Context,
	streamURL string,
	gain float64,
	primeSilenceMs int,
) (*exec.Cmd, io.ReadCloser, io.ReadCloser, error) {
	// Audio filter chain: dynaudnorm (real-time normalization with ~1s
	// latency) + volume (camera gain) + optional prime silence (adelay).
	// dynaudnorm normalizes audio levels so quiet streams (e.g. ATC) are
	// audible without manual gain adjustment. We use dynaudnorm instead of
	// loudnorm because loudnorm has a 3+ second buffer that causes the
	// camera to timeout and close the connection.
	af := fmt.Sprintf("dynaudnorm=f=150:g=15:p=0.9,volume=%.2f", gain)
	if primeSilenceMs > 0 {
		af = fmt.Sprintf(
			"adelay=%d|%d,dynaudnorm=f=150:g=15:p=0.9,volume=%.2f",
			primeSilenceMs,
			primeSilenceMs,
			gain,
		)
	}

	cmd := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-nostdin",
		"-loglevel", "info", // info level for ICY metadata
		"-re", // read input at native frame rate for live streams
		"-user_agent",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36",
		"-i",
		streamURL,
		"-af",
		af,
		"-acodec",
		"pcm_mulaw",
		"-ar",
		"8000",
		"-ac",
		"1",
		"-f",
		"mulaw",
		"-",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("ffmpeg stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("ffmpeg stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, nil, fmt.Errorf("starting ffmpeg: %w", err)
	}
	return cmd, stdout, stderr, nil
}

// startStreamToCamera starts an ffmpeg process reading a live stream URL and
// piping transcoded G.711 µ-law to the camera speaker. It registers a
// streamSession so the stream can be paused, resumed, and stopped. The
// originalURL is used for logging and playback state (before playlist
// resolution). This is shared by the /api/play-stream handler and the
// stream-preset playback path in playPreset().
//
// A supervisor goroutine handles automatic reconnection with exponential
// backoff if the stream drops (network blip, server restart). ICY metadata
// from the stream is parsed and shown in the playback detail.
func (h *Handlers) startStreamToCamera(
	log *clog.Logger,
	cam cameras.Speaker,
	cameraName, streamURL, originalURL string,
	reqGain float64,
) error {
	gain := h.effectiveGain(cameraName, reqGain)

	// Stop any existing ffmpeg stream for this camera first.
	stopStream(cameraName)

	ctx, cancel := context.WithCancel(context.Background())

	session := &streamSession{
		cancel:  cancel,
		url:     originalURL,
		started: now(),
	}

	activeStreamsMu.Lock()
	activeStreams[cameraName] = session
	activeStreamsMu.Unlock()

	setPlayback(cameraName, "stream", originalURL)

	// Start the stream supervisor — handles reconnection on stream drop.
	go h.streamSupervisor(log, cam, cameraName, streamURL, gain, ctx, session)

	log.Info("stream: started", "camera", cameraName, "url", originalURL)
	h.events.publish(
		event{Camera: cameraName, Action: "play-stream", Text: originalURL, At: now()},
	)
	return nil
}

// streamSupervisor manages the ffmpeg → camera pipeline lifecycle. If ffmpeg
// exits (stream drop) or the camera closes the connection, it retries with
// exponential backoff up to maxRetries times. User-initiated stop (via
// context cancellation) exits immediately without retry.
func (h *Handlers) streamSupervisor(
	log *clog.Logger,
	cam cameras.Speaker,
	cameraName, streamURL string,
	gain float64,
	ctx context.Context,
	session *streamSession,
) {
	backoff := 2 * time.Second
	const maxBackoff = 32 * time.Second
	const maxRetries = 5
	// Reset the retry counter if the stream ran for this long before dropping.
	const successResetThreshold = 30 * time.Second
	retries := 0

	for {
		select {
		case <-ctx.Done():
			stopStream(cameraName)
			return
		default:
		}

		cmd, stdout, stderr, err := buildStreamFFmpegCmd(
			ctx, streamURL, gain, h.cfg.PrimeSilenceMs,
		)
		if err != nil {
			log.Warn("stream: ffmpeg failed to start", "camera", cameraName, "err", err)
			stopStream(cameraName)
			return
		}

		// Update session with new cmd (for pause/resume/stop).
		activeStreamsMu.Lock()
		session.cmd = cmd
		activeStreamsMu.Unlock()

		// Wrap stdout with a level tap for VU meter sampling.
		tap := &levelTapReader{r: stdout, session: session}

		go logStderr(stderr, log, cameraName)

		// Start camera stream in a goroutine — interrupt AirPlay first.
		camDone := make(chan struct{})
		go func() {
			_ = cam.Stop()
			_ = cam.Stream(tap)
			close(camDone)
		}()

		// Wait for ffmpeg to exit in a separate goroutine.
		ffmpegDone := make(chan error, 1)
		go func() {
			ffmpegDone <- cmd.Wait()
		}()

		// Wait for either side to finish, or user stop.
		var ffmpegErr error
		streamStart := time.Now() // tracked for success-reset threshold
		select {
		case <-ctx.Done():
			// User stop — kill ffmpeg, wait for both to finish.
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			<-ffmpegDone
			<-camDone
			stopStream(cameraName)
			log.Info("stream: stopped", "camera", cameraName)
			return
		case ffmpegErr = <-ffmpegDone:
			// ffmpeg exited (stream drop) — wait for cam.Stream to finish
			// (it will get EOF from the tap reader).
			<-camDone
		case <-camDone:
			// Camera closed connection — kill ffmpeg and reap.
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			ffmpegErr = <-ffmpegDone
		}

		// If the stream ran long enough, reset the retry counter and
		// backoff — this was a healthy session that dropped, not a
		// persistent failure.
		if elapsed := time.Since(streamStart); elapsed >= successResetThreshold {
			if retries > 0 {
				log.Debug("stream: resetting retry counter after stable session",
					"camera", cameraName, "elapsed", elapsed, "prev_retries", retries)
			}
			retries = 0
			backoff = 2 * time.Second
		}

		// Stream dropped — attempt reconnection.
		if retries >= maxRetries {
			log.Warn("stream: reconnect attempts exhausted, stopping",
				"camera", cameraName, "retries", retries, "err", ffmpegErr)
			stopStream(cameraName)
			return
		}
		retries++
		log.Warn("stream: dropped, reconnecting",
			"camera", cameraName, "attempt", retries, "backoff", backoff, "err", ffmpegErr)

		select {
		case <-ctx.Done():
			stopStream(cameraName)
			return
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func logStderr(stderr io.ReadCloser, log *clog.Logger, camera string) {
	defer stderr.Close()
	scanner := bufio.NewScanner(stderr)
	// Track the highest metadata priority seen so far so that
	// icy-description doesn't overwrite a more specific icy-name or
	// StreamTitle, and icy-name doesn't overwrite an ongoing StreamTitle.
	var bestKind icyKind
	for scanner.Scan() {
		line := scanner.Text()
		// Parse ICY metadata from ffmpeg stderr (requires -loglevel info).
		if title, kind := parseICYMetadata(line); kind != icyNone {
			if kind >= bestKind {
				updatePlaybackDetail(camera, title)
				bestKind = kind
				log.Debug("stream: icy metadata", "camera", camera, "title", title)
			}
			continue
		}
		log.Debug("stream: ffmpeg", "camera", camera, "stderr", line)
	}
}

// parseICYMetadata extracts metadata from an ffmpeg stderr line. Handles
// three formats:
//   - ICY Info: StreamTitle='Artist - Title';  (music streams, ongoing updates)
//   - icy-name        : Station Name            (ATC/other streams, once at startup)
//   - icy-description : Description             (fallback if icy-name absent)
//
// Returns the parsed value and the kind of metadata found.
// Returns empty string and kindNone if the line doesn't contain parseable
// metadata.
func parseICYMetadata(line string) (value string, kind icyKind) {
	// Format 1: ICY Info: StreamTitle='...'
	if idx := strings.Index(line, "ICY Info: StreamTitle='"); idx >= 0 {
		start := idx + len("ICY Info: StreamTitle='")
		end := strings.Index(line[start:], "'")
		if end >= 0 {
			return line[start : start+end], icyTitle
		}
	}
	// Format 2/3: icy-name / icy-description (indented, appears in Metadata
	// block at startup).
	trimmed := strings.TrimSpace(line)
	for _, prefix := range []struct {
		str  string
		kind icyKind
	}{
		{"icy-name", icyName},
		{"icy-description", icyDescription},
	} {
		if strings.HasPrefix(trimmed, prefix.str) {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), prefix.kind
			}
		}
	}
	return "", icyNone
}

// icyKind classifies the metadata source for priority resolution.
type icyKind int

const (
	icyNone        icyKind = iota
	icyDescription         // icy-description: lowest priority (fallback)
	icyName                // icy-name: medium priority (station name)
	icyTitle               // StreamTitle: highest priority (ongoing updates)
)

// stopStream kills the active ffmpeg stream for camera, if any.
func stopStream(camera string) {
	activeStreamsMu.Lock()
	sess := activeStreams[camera]
	delete(activeStreams, camera)
	if sess != nil {
		sess.cancel()
		if sess.cmd != nil && sess.cmd.Process != nil {
			_ = sess.cmd.Process.Kill()
		}
	}
	activeStreamsMu.Unlock()
	if sess != nil {
		clearPlayback(camera)
	}
}

// stopAllStreams kills every active ffmpeg stream.
func stopAllStreams() {
	activeStreamsMu.Lock()
	sessions := make(map[string]*streamSession, len(activeStreams))
	for k, v := range activeStreams {
		sessions[k] = v
	}
	activeStreams = make(map[string]*streamSession)
	for _, sess := range sessions {
		sess.cancel()
		if sess.cmd != nil && sess.cmd.Process != nil {
			_ = sess.cmd.Process.Kill()
		}
	}
	activeStreamsMu.Unlock()
}

// pauseStream suspends the ffmpeg process for a camera's active stream by
// sending SIGSTOP. The camera speaker connection stays open; ffmpeg's read
// position is preserved. Returns the stream URL and whether it was already
// paused. ok is false when there is no active stream for the camera.
func pauseStream(camera string) (url string, alreadyPaused, ok bool) {
	activeStreamsMu.Lock()
	defer activeStreamsMu.Unlock()
	sess := activeStreams[camera]
	if sess == nil {
		return "", false, false
	}
	if sess.paused {
		return sess.url, true, true
	}
	if sess.cmd != nil && sess.cmd.Process != nil {
		_ = sess.cmd.Process.Signal(syscall.SIGSTOP)
	}
	sess.paused = true
	return sess.url, false, true
}

// resumeStream resumes a paused ffmpeg process via SIGCONT. Returns the stream
// URL and whether it was not paused. ok is false when there is no active
// stream for the camera.
func resumeStream(camera string) (url string, notPaused, ok bool) {
	activeStreamsMu.Lock()
	defer activeStreamsMu.Unlock()
	sess := activeStreams[camera]
	if sess == nil {
		return "", false, false
	}
	if !sess.paused {
		return sess.url, true, true
	}
	if sess.cmd != nil && sess.cmd.Process != nil {
		_ = sess.cmd.Process.Signal(syscall.SIGCONT)
	}
	sess.paused = false
	return sess.url, false, true
}

// pauseAllStreams suspends every active stream. Returns the list of camera
// names that were paused (excluding already-paused ones).
func pauseAllStreams() []string {
	activeStreamsMu.Lock()
	names := make([]string, 0, len(activeStreams))
	for name, sess := range activeStreams {
		if sess.paused {
			continue
		}
		if sess.cmd != nil && sess.cmd.Process != nil {
			_ = sess.cmd.Process.Signal(syscall.SIGSTOP)
		}
		sess.paused = true
		names = append(names, name)
	}
	activeStreamsMu.Unlock()
	return names
}

// resumeAllStreams resumes every paused stream. Returns the list of camera
// names that were resumed.
func resumeAllStreams() []string {
	activeStreamsMu.Lock()
	names := make([]string, 0, len(activeStreams))
	for name, sess := range activeStreams {
		if !sess.paused {
			continue
		}
		if sess.cmd != nil && sess.cmd.Process != nil {
			_ = sess.cmd.Process.Signal(syscall.SIGCONT)
		}
		sess.paused = false
		names = append(names, name)
	}
	activeStreamsMu.Unlock()
	return names
}

func now() time.Time {
	return time.Now()
}

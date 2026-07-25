package api

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os/exec"
	"strings"
	"sync"
	"time"

	clog "github.com/charmbracelet/log"
	"github.com/labstack/echo/v4"
)

// streamSession tracks a live ffmpeg → camera stream so it can be stopped on demand.
type streamSession struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
}

var (
	activeStreams   = make(map[string]*streamSession)
	activeStreamsMu sync.Mutex
)

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

func resolvePLS(rawURL string) (string, error) {
	resp, err := http.Get(rawURL)
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
			return strings.TrimPrefix(line, "File1="), nil
		}
		// Case-insensitive fallback.
		if strings.Contains(strings.ToLower(line), "file1=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
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

	resp, err := http.Get(rawURL)
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

	gain := req.Gain
	if gain <= 0 {
		gain = 3.0
	}

	streamURL, err := resolveStreamURL(req.URL)
	if err != nil {
		log.Warn("stream: failed to resolve playlist", "url", req.URL, "err", err)
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}

	// Stop any existing stream for this camera first.
	stopStream(req.Camera)

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-nostdin", "-loglevel", "error",
		"-re", // read input at native frame rate for live streams
		"-i", streamURL,
		"-af", fmt.Sprintf("volume=%.2f", gain),
		"-acodec", "pcm_mulaw",
		"-ar", "8000",
		"-ac", "1",
		"-f", "mulaw",
		"-",
	)
	// ffmpeg outputs raw mu-law to stdout.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	activeStreamsMu.Lock()
	activeStreams[req.Camera] = &streamSession{cmd: cmd, cancel: cancel}
	activeStreamsMu.Unlock()

	go logStderr(stderr, log, req.Camera)
	go func() {
		_ = cam.Stream(stdout)
		// Camera side finished; clean up ffmpeg.
		stopStream(req.Camera)
	}()

	// Don't block waiting for the stream; return immediately.
	go func() {
		_ = cmd.Wait()
		stopStream(req.Camera)
	}()

	log.Info("stream: started", "camera", req.Camera, "url", req.URL)
	h.events.publish(event{Camera: req.Camera, Action: "play-stream", Text: req.URL, At: now()})
	return c.JSON(http.StatusOK, map[string]string{"status": "streaming"})
}

func logStderr(stderr io.ReadCloser, log *clog.Logger, camera string) {
	defer stderr.Close()
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		log.Debug("stream: ffmpeg", "camera", camera, "stderr", scanner.Text())
	}
}

// stopStream kills the active ffmpeg stream for camera, if any.
func stopStream(camera string) {
	activeStreamsMu.Lock()
	sess := activeStreams[camera]
	delete(activeStreams, camera)
	activeStreamsMu.Unlock()
	if sess == nil {
		return
	}
	sess.cancel()
	if sess.cmd.Process != nil {
		_ = sess.cmd.Process.Kill()
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
	activeStreamsMu.Unlock()
	for _, sess := range sessions {
		sess.cancel()
		if sess.cmd.Process != nil {
			_ = sess.cmd.Process.Kill()
		}
	}
}

func now() time.Time {
	return time.Now()
}

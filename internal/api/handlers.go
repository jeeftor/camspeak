package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	neturl "net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	clog "github.com/charmbracelet/log"
	"github.com/labstack/echo/v4"

	"github.com/jeeftor/camspeak/internal/airplay"
	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/library"
	"github.com/jeeftor/camspeak/internal/tts"
	"github.com/jeeftor/camspeak/internal/util"
	"github.com/jeeftor/camspeak/internal/vision"
)

// Handlers holds all route handler dependencies.
type Handlers struct {
	cfg          *config.Config
	cfgMu        sync.Mutex
	configEditMu sync.Mutex
	reg          *cameras.Registry
	airplayMgr   *airplay.Manager
	store        *library.Store
	tts          *tts.Client
	vision       *vision.Client
	events       *eventBus
	db           *sql.DB
	tmpDir       string
	log          *clog.Logger
	uploads      uploadWorker
	describes    describeJobs
}

// configSnapshot returns configuration values without exposing the mutable camera map.
func (h *Handlers) configSnapshot() config.Config {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	cfg := *h.cfg
	cfg.Cameras = maps.Clone(h.cfg.Cameras)
	return cfg
}

// ttsClient returns the current immutable client, which settings updates replace atomically.
func (h *Handlers) ttsClient() *tts.Client {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	return h.tts
}

// visionClient returns the current immutable vision client.
func (h *Handlers) visionClient() *vision.Client {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	return h.vision
}

// SetAirPlayManager attaches a live AirPlay manager so per-camera toggles
// take effect immediately without a restart.
func (h *Handlers) SetAirPlayManager(m *airplay.Manager) {
	h.airplayMgr = m
}

// logger returns the handler logger augmented with the Echo request ID.
func (h *Handlers) logger(c echo.Context) *clog.Logger {
	if rid := c.Response().Header().Get(echo.HeaderXRequestID); rid != "" {
		return h.log.With("request_id", rid)
	}
	return h.log
}

// speakReq is the body for POST /api/speak.
type speakReq struct {
	Camera string   `json:"camera"`
	Text   string   `json:"text"`
	Voice  string   `json:"voice"`
	Gain   *float64 `json:"gain"`
}

// playReq is the body for POST /api/play.
type playReq struct {
	Camera   string   `json:"camera"`
	Preset   string   `json:"preset"`
	Category string   `json:"category"`
	Gain     *float64 `json:"gain"`
	Loop     int      `json:"loop"`
}

// broadcastReq is the body for POST /api/broadcast.
type broadcastReq struct {
	Text     string   `json:"text"`
	Preset   string   `json:"preset"`
	Category string   `json:"category"`
	Voice    string   `json:"voice"`
	Gain     *float64 `json:"gain"`
	Loop     int      `json:"loop"`
}

// genPresetReq is the body for POST /api/library.
// A preset is either a TTS clip (Text + Voice) or a live stream (URL).
type genPresetReq struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Text     string `json:"text"`
	Voice    string `json:"voice"`
	URL      string `json:"url"`
}

// requestGain distinguishes an omitted per-call override from an explicit mute.
func requestGain(gain *float64) float64 {
	if gain == nil {
		return -1
	}
	return *gain
}

// validateRequestGain checks the public gain range before starting playback work.
func validateRequestGain(gain *float64) error {
	if gain != nil && (*gain < 0 || *gain > 10) {
		return echo.NewHTTPError(http.StatusBadRequest, "gain must be between 0 and 10")
	}
	return nil
}

// Speak handles POST /api/speak — TTS → camera.
func (h *Handlers) Speak(c echo.Context) error {
	log := h.logger(c)

	var req speakReq
	err := c.Bind(&req)
	if err != nil || req.Camera == "" || req.Text == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "camera and text required")
	}
	if err := validateRequestGain(req.Gain); err != nil {
		return err
	}

	log.Info(
		"speak: request",
		"camera",
		req.Camera,
		"text_len",
		len(req.Text),
		"voice",
		req.Voice,
		"gain",
		req.Gain,
	)
	start := time.Now()

	timings, err := h.speakTextContext(
		c.Request().Context(),
		log,
		req.Camera,
		req.Text,
		req.Voice,
		requestGain(req.Gain),
	)
	if err != nil {
		log.Error("speak: failed", "camera", req.Camera, "elapsed", time.Since(start), "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	log.Info(
		"speak: done",
		"camera",
		req.Camera,
		"elapsed",
		time.Since(start),
		"ttfs_ms",
		timings.TTFS(),
	)
	return c.JSON(http.StatusOK, map[string]any{
		"status":   "ok",
		"timings":  timings.Ms(),
		"tts_mode": timings.TTSMode,
		"ttfs_ms":  timings.TTFS(),
		"total_ms": TotalMs(start),
	})
}

// Play handles POST /api/play — preset → camera.
func (h *Handlers) Play(c echo.Context) error {
	log := h.logger(c)

	var req playReq
	err := c.Bind(&req)
	if err != nil || req.Camera == "" || req.Preset == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "camera and preset required")
	}
	if err := validateRequestGain(req.Gain); err != nil {
		return err
	}
	if req.Loop < -1 {
		return echo.NewHTTPError(http.StatusBadRequest, "loop must be -1 or a nonnegative integer")
	}

	log.Info(
		"play: request",
		"camera",
		req.Camera,
		"preset",
		req.Preset,
		"category",
		req.Category,
		"gain",
		req.Gain,
		"loop",
		req.Loop,
	)
	start := time.Now()

	timings, err := h.playPresetContext(
		c.Request().Context(),
		log,
		req.Camera,
		req.Category,
		req.Preset,
		requestGain(req.Gain),
		req.Loop,
	)
	if err != nil {
		log.Error(
			"play: failed",
			"camera",
			req.Camera,
			"preset",
			req.Preset,
			"elapsed",
			time.Since(start),
			"err",
			err,
		)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	log.Info(
		"play: done",
		"camera",
		req.Camera,
		"preset",
		req.Preset,
		"elapsed",
		time.Since(start),
	)
	return c.JSON(http.StatusOK, map[string]any{
		"status":   "ok",
		"timings":  timings.Ms(),
		"ttfs_ms":  timings.TTFS(),
		"total_ms": TotalMs(start),
	})
}

// playURLError is a structured error that lets the REST handler map to the
// right HTTP status code while still exposing a plain message for the MCP tool.
type playURLError struct {
	status int
	msg    string
}

func (e *playURLError) Error() string { return e.msg }

// doPlayURLContext owns download and conversion as part of the cancelable playback operation.
func (h *Handlers) doPlayURLContext(
	ctx context.Context,
	log *clog.Logger,
	camera, rawURL string,
	gain float64,
) error {
	// Validate URL scheme to prevent SSRF (only http/https allowed), and
	// derive a redacted URL for logging/event storage.
	parsedURL, err := neturl.Parse(rawURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return &playURLError{http.StatusBadRequest, "url must be http or https"}
	}

	redactedURL := util.RedactURL(parsedURL)

	log.Info(
		"play-url: request",
		"camera",
		camera,
		"url",
		redactedURL,
		"gain",
		h.effectiveGain(camera, gain),
	)
	start := time.Now()

	cam, err := h.reg.GetForPlayback(camera)
	if err != nil {
		return &playURLError{http.StatusNotFound, err.Error()}
	}

	op, err := h.beginPlaybackOperation(ctx, camera, cam, "play-url", redactedURL)
	if err != nil {
		return &playURLError{http.StatusConflict, err.Error()}
	}
	defer op.finish()
	download, err := http.NewRequestWithContext(op.ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return &playURLError{http.StatusBadRequest, "invalid audio URL"}
	}
	resp, err := (&http.Client{Timeout: 2 * time.Minute}).Do(download)
	if err != nil {
		log.Error("play-url: download failed", "camera", camera, "url", redactedURL, "err", err)
		return &playURLError{http.StatusBadGateway, fmt.Sprintf("download failed: %s", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Error(
			"play-url: download bad status",
			"camera", camera,
			"url", redactedURL,
			"status", resp.StatusCode,
		)
		return &playURLError{
			http.StatusBadGateway,
			fmt.Sprintf("download returned HTTP %d", resp.StatusCode),
		}
	}

	tmp, err := os.CreateTemp(h.tmpDir, "camspeak_url_*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	const maxAudioDownload = 64 << 20
	if n, err := io.Copy(tmp, io.LimitReader(resp.Body, maxAudioDownload+1)); err != nil {
		tmp.Close()
		return fmt.Errorf("saving download: %w", err)
	} else if n > maxAudioDownload {
		tmp.Close()
		return &playURLError{http.StatusRequestEntityTooLarge, "audio download exceeds 64 MiB"}
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}

	// Transcode to raw
	raw, err := os.CreateTemp(h.tmpDir, "camspeak_url_*.raw")
	if err != nil {
		return err
	}
	rawName := raw.Name()
	raw.Close()

	if err := transcodeFileToRawGainWithPrimeContext(op.ctx, tmpName, rawName, 1.0, h.configSnapshot().PrimeSilenceMs); err != nil {
		os.Remove(rawName)
		return err
	}

	defer os.Remove(rawName)

	log.Debug("play-url: sending to camera", "camera", camera, "url", redactedURL)
	if _, err := op.send(rawName, h.gainForCall(camera, gain)); err != nil {
		log.Error(
			"play-url: send failed",
			"camera", camera,
			"elapsed", time.Since(start),
			"err", err,
		)
		return err
	}

	log.Info(
		"play-url: done",
		"camera", camera,
		"url", redactedURL,
		"elapsed", time.Since(start),
	)
	h.events.publish(
		event{
			Camera: camera, Action: "play-url", Text: redactedURL, At: time.Now(),
			Replay: playbackReplay(
				"/api/play-url",
				map[string]any{"camera": camera, "url": rawURL},
				gain,
			),
		},
	)

	return nil
}

// PlayURL handles POST /api/play-url — download URL → transcode → camera.
func (h *Handlers) PlayURL(c echo.Context) error {
	var req struct {
		Camera string   `json:"camera"`
		URL    string   `json:"url"`
		Gain   *float64 `json:"gain"`
	}
	if err := c.Bind(&req); err != nil || req.Camera == "" || req.URL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "camera and url required")
	}
	if err := validateRequestGain(req.Gain); err != nil {
		return err
	}

	if err := h.doPlayURLContext(c.Request().Context(), h.logger(c), req.Camera, req.URL, requestGain(req.Gain)); err != nil {
		var perr *playURLError
		if errors.As(err, &perr) {
			return echo.NewHTTPError(perr.status, perr.msg)
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// Stop handles POST /api/stop — stops audio on a specific camera or all cameras.
// If the request body contains a "camera" field, only that camera is stopped.
// Otherwise (empty body or no camera field), all cameras are stopped.
// For each affected camera this also kills any live stream and restarts the
// AirPlay receiver so the camera is fully reset.
func (h *Handlers) Stop(c echo.Context) error {
	log := h.logger(c)

	var req struct {
		Camera string `json:"camera"`
	}
	// An empty body means stop all; malformed input must never broaden scope.
	if err := c.Bind(&req); err != nil {
		log.Warn("stop: invalid request; playback unchanged")
		return echo.NewHTTPError(http.StatusBadRequest, "invalid stop request")
	}
	started := time.Now()
	log.Info("stop: requested", "camera", req.Camera, "all", req.Camera == "")

	if req.Camera != "" {
		stopOperation(req.Camera)
		stopStream(req.Camera)
		if err := h.reg.Stop(req.Camera); err != nil {
			log.Warn("stop: camera not found", "camera", req.Camera, "err", err)
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		clearPlayback(req.Camera)
		h.resetAirPlay(req.Camera, log)
		log.Info(
			"stop: camera stop completed",
			"camera",
			req.Camera,
			"duration_ms",
			time.Since(started).Milliseconds(),
		)
		h.events.publish(event{Camera: req.Camera, Action: "stop", At: time.Now()})
		return c.JSON(http.StatusOK, map[string]string{"status": "stopped", "camera": req.Camera})
	}

	// Stop all cameras, live streams, and reset AirPlay receivers.
	stopAllOperations()
	stopAllStreams()
	h.reg.StopAll()
	clearAllPlayback()
	h.resetAllAirPlay(log)
	log.Info("stop: all camera stops completed", "duration_ms", time.Since(started).Milliseconds())
	h.events.publish(event{Action: "stop-all", At: time.Now()})
	return c.JSON(http.StatusOK, map[string]string{"status": "stopped", "camera": "all"})
}

// Pause handles POST /api/pause — suspends a live stream on a specific camera
// or all cameras. Unlike /api/stop, this keeps the ffmpeg process and the
// camera speaker connection alive; the transcoder is frozen via SIGSTOP so
// playback can be resumed in place with /api/resume. Only affects streams
// started via /api/play-stream.
func (h *Handlers) Pause(c echo.Context) error {
	log := h.logger(c)

	var req struct {
		Camera string `json:"camera"`
	}
	if err := c.Bind(&req); err != nil {
		log.Warn("pause: invalid request; playback unchanged")
		return echo.NewHTTPError(http.StatusBadRequest, "invalid pause request")
	}

	if req.Camera != "" {
		url, alreadyPaused, ok := pauseStream(req.Camera)
		if !ok {
			log.Warn("pause: no active stream", "camera", req.Camera)
			return echo.NewHTTPError(
				http.StatusNotFound,
				fmt.Sprintf("no active stream for camera %s", req.Camera),
			)
		}
		status := "paused"
		if alreadyPaused {
			status = "already-paused"
		}
		setPlaybackPaused(req.Camera, true)
		log.Info("pause: stream", "camera", req.Camera, "status", status, "url", url)
		h.events.publish(event{Camera: req.Camera, Action: "pause", Text: url, At: time.Now()})
		return c.JSON(http.StatusOK, map[string]string{"status": status, "camera": req.Camera})
	}

	paused := pauseAllStreams()
	for _, name := range paused {
		setPlaybackPaused(name, true)
	}
	log.Info("pause: all streams", "count", len(paused), "cameras", strings.Join(paused, ","))
	for _, name := range paused {
		h.events.publish(event{Camera: name, Action: "pause", At: time.Now()})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": "paused", "cameras": paused})
}

// Resume handles POST /api/resume — resumes a paused live stream on a specific
// camera or all cameras via SIGCONT. Only affects streams started via
// /api/play-stream that were previously paused with /api/pause.
func (h *Handlers) Resume(c echo.Context) error {
	log := h.logger(c)

	var req struct {
		Camera string `json:"camera"`
	}
	if err := c.Bind(&req); err != nil {
		log.Warn("resume: invalid request; playback unchanged")
		return echo.NewHTTPError(http.StatusBadRequest, "invalid resume request")
	}

	if req.Camera != "" {
		url, notPaused, ok := resumeStream(req.Camera)
		if !ok {
			log.Warn("resume: no active stream", "camera", req.Camera)
			return echo.NewHTTPError(
				http.StatusNotFound,
				fmt.Sprintf("no active stream for camera %s", req.Camera),
			)
		}
		status := "resumed"
		if notPaused {
			status = "not-paused"
		}
		setPlaybackPaused(req.Camera, false)
		log.Info("resume: stream", "camera", req.Camera, "status", status, "url", url)
		h.events.publish(event{Camera: req.Camera, Action: "resume", Text: url, At: time.Now()})
		return c.JSON(http.StatusOK, map[string]string{"status": status, "camera": req.Camera})
	}

	resumed := resumeAllStreams()
	for _, name := range resumed {
		setPlaybackPaused(name, false)
	}
	log.Info("resume: all streams", "count", len(resumed), "cameras", strings.Join(resumed, ","))
	for _, name := range resumed {
		h.events.publish(event{Camera: name, Action: "resume", At: time.Now()})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": "resumed", "cameras": resumed})
}

// resetAirPlay restarts the AirPlay receiver for a single camera, if AirPlay is configured.
func (h *Handlers) resetAirPlay(name string, log *clog.Logger) {
	if h.airplayMgr == nil {
		return
	}
	if !h.airplayMgr.IsRunning(name) {
		log.Debug("stop: AirPlay reset skipped; receiver not running", "camera", name)
		return
	}
	started := time.Now()
	log.Info("stop: restarting AirPlay receiver", "camera", name)
	h.airplayMgr.Disable(name)
	if err := h.airplayMgr.Enable(name); err != nil {
		log.Warn("stop: AirPlay reset failed", "camera", name, "err", err)
		return
	}
	log.Info(
		"stop: AirPlay receiver restarted; waiting for sender audio",
		"camera",
		name,
		"duration_ms",
		time.Since(started).Milliseconds(),
	)
}

// resetAllAirPlay restarts every running AirPlay receiver.
func (h *Handlers) resetAllAirPlay(log *clog.Logger) {
	if h.airplayMgr == nil {
		return
	}
	for name, running := range h.airplayMgr.Status() {
		if !running {
			continue
		}
		h.resetAirPlay(name, log)
	}
}

// Beep handles POST /api/beep — 800Hz test tone → camera.
func (h *Handlers) Beep(c echo.Context) error {
	log := h.logger(c)

	var req struct {
		Camera string `json:"camera"`
	}
	if err := c.Bind(&req); err != nil || req.Camera == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "camera required")
	}

	// A deliberate diagnostic beep is permitted without enabling normal playback.
	cam, err := h.reg.Get(req.Camera)
	if err != nil {
		log.Warn("beep: camera not found", "camera", req.Camera, "err", err)
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	op, err := h.beginPlaybackOperation(
		c.Request().Context(),
		req.Camera,
		cam,
		"beep",
		"800Hz test tone",
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	}
	defer op.finish()
	raw, err := GenerateBeepContext(op.ctx, h.tmpDir)
	if err != nil {
		log.Error("beep: generating tone failed", "camera", req.Camera, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer os.Remove(raw)

	log.Info(
		"beep: sending",
		"camera",
		req.Camera,
		"type",
		h.configSnapshot().Cameras[req.Camera].Type,
	)
	start := time.Now()

	if _, err := op.send(raw, h.gainForCall(req.Camera, -1)); err != nil {
		log.Error(
			"beep: send failed",
			"camera",
			req.Camera,
			"elapsed",
			time.Since(start),
			"err",
			err,
		)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	log.Info("beep: sent", "camera", req.Camera, "elapsed", time.Since(start))
	h.events.publish(event{Camera: req.Camera, Action: "beep", At: time.Now()})

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// Cameras handles GET /api/cameras — returns only enabled cameras.
func (h *Handlers) Cameras(c echo.Context) error {
	status := h.reg.Status()
	config := h.configSnapshot()
	go2rtcURL, _ := h.reg.Routing()

	out := make([]map[string]any, 0)
	for _, name := range h.sortedCameraNames() {
		cfg := config.Cameras[name]
		if !cfg.Enabled {
			continue
		}
		live := cfg.Type == "hikvision"
		speak := cfg.Type == "hikvision" || cfg.Type == "onvif" ||
			((cfg.Type == "go2rtc" || cfg.Type == "reolink") && cfg.Stream != "" && go2rtcURL != "")
		snapshot := cfg.Type == "hikvision" || cfg.Type == "reolink" || config.FrigateURL != "" ||
			(config.Go2rtcURL != "" && (cfg.Stream != "" || cfg.VisionStream != ""))
		out = append(out, map[string]any{
			"name":            name,
			"type":            cfg.Type,
			"ip":              cfg.IP,
			"online":          status[name],
			"vision_prompt":   cfg.VisionPrompt,
			"vision_stream":   cfg.VisionStream,
			"vision_width":    cfg.VisionWidth,
			"snap_method":     cfg.SnapMethod,
			"note":            cfg.Note,
			"airplay_enabled": cfg.AirPlayEnabled,
			"airplay_name":    cfg.AirPlayName,
			"airplay_model":   cfg.AirPlayModel,
			"sort_order":      cfg.SortOrder,
			"gain":            cfg.Gain,
			"capabilities": map[string]bool{
				"speak": speak, "snapshot": snapshot, "live_stream": live, "airplay": live,
			},
		})
	}

	return c.JSON(http.StatusOK, out)
}

// Playback handles GET /api/playback — returns the current audio playback
// state for every enabled camera. Each entry reports state ("playing",
// "paused", or "idle"), the source action ("stream", "speak", "play",
// "play-url", "beep"), a human-readable detail string (URL, text, preset
// name), and timestamps for when playback started and (if paused) when it
// was paused.
func (h *Handlers) Playback(c echo.Context) error {
	config := h.configSnapshot()
	names := make([]string, 0, len(config.Cameras))
	for name, cfg := range config.Cameras {
		if cfg.Enabled {
			names = append(names, name)
		}
	}
	return c.JSON(http.StatusOK, getAllPlayback(names))
}

// Voices handles GET /api/voices.
func (h *Handlers) Voices(c echo.Context) error {
	return c.JSON(http.StatusOK, h.ttsClient().Voices())
}

// Health handles GET /api/health.
func (h *Handlers) Health(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"status":  "ok",
		"version": Version,
	})
}

// Events handles GET /api/events — SSE stream of speak events.
func (h *Handlers) Events(c echo.Context) error {
	log := h.logger(c)

	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().WriteHeader(http.StatusOK)
	ch := h.events.subscribe()
	defer h.events.unsubscribe(ch)

	// Send recent history on connect
	if recent, err := h.events.recentEvents(50); err == nil {
		for _, v := range slices.Backward(recent) {
			data, err := json.Marshal(v)
			if err != nil {
				log.Error("events: marshal recent failed", "err", err)
				continue
			}
			fmt.Fprintf(c.Response(), "data: %s\n\n", data)
		}

		c.Response().Flush()
	}

	for {
		select {
		case ev := <-ch:
			data, err := json.Marshal(ev)
			if err != nil {
				log.Error("events: marshal event failed", "err", err)
				continue
			}
			fmt.Fprintf(c.Response(), "data: %s\n\n", data)
			c.Response().Flush()
		case <-c.Request().Context().Done():
			return nil
		}
	}
}

// StreamLevels handles GET /api/stream-levels — SSE stream of audio levels
// for active streams, pushed at ~10fps. Each event is a JSON object mapping
// camera names to level values (0.0–1.0). Only cameras with active streams
// are included. When no streams are active, periodic empty events are sent
// so the client knows the connection is alive.
func (h *Handlers) StreamLevels(c echo.Context) error {
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().WriteHeader(http.StatusOK)

	ticker := time.NewTicker(100 * time.Millisecond) // 10fps
	defer ticker.Stop()

	// Send an initial empty event so the client gets a response immediately.
	fmt.Fprintf(c.Response(), "data: {}\n\n")
	c.Response().Flush()

	for {
		select {
		case <-ticker.C:
			levels := getStreamLevels()
			data, err := json.Marshal(levels)
			if err != nil {
				continue
			}
			fmt.Fprintf(c.Response(), "data: %s\n\n", data)
			c.Response().Flush()
		case <-c.Request().Context().Done():
			return nil
		}
	}
}

// PingCamera handles POST /api/cameras/:name/ping — checks if the camera is reachable.
func (h *Handlers) PingCamera(c echo.Context) error {
	name := c.Param("name")
	h.cfgMu.Lock()
	_, ok := h.cfg.Cameras[name]
	h.cfgMu.Unlock()
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "camera not found")
	}
	cam, err := h.reg.Get(name)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "camera not loaded")
	}
	ok = cam.Ping()
	if ok {
		return c.JSON(http.StatusOK, map[string]interface{}{"ok": true, "camera": name})
	}
	return c.JSON(
		http.StatusOK,
		map[string]interface{}{"ok": false, "camera": name, "error": "unreachable"},
	)
}

// EventLog handles GET /api/events/log — returns historical events as JSON.
func (h *Handlers) EventLog(c echo.Context) error {
	limit := 100
	if l := c.QueryParam("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil {
			limit = n
		}
	}
	camera := c.QueryParam("camera")

	events, err := h.events.queryEvents(limit, camera)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if events == nil {
		events = []event{}
	}
	return c.JSON(http.StatusOK, events)
}

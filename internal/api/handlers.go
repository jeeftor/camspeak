package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"slices"
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
	cfg             *config.Config
	cfgMu           sync.Mutex
	reg             *cameras.Registry
	airplayMgr      *airplay.Manager
	store           *library.Store
	tts             *tts.Client
	vision          *vision.Client
	events          *eventBus
	mqttMsgBus      *mqttMsgBus
	mqttBroker      string
	mqttStatusFn    func() string
	mqttSubscribeFn func(string) error
	db              *sql.DB
	tmpDir          string
	log             *clog.Logger
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
	Camera string  `json:"camera"`
	Text   string  `json:"text"`
	Voice  string  `json:"voice"`
	Gain   float64 `json:"gain"`
}

// playReq is the body for POST /api/play.
type playReq struct {
	Camera   string  `json:"camera"`
	Preset   string  `json:"preset"`
	Category string  `json:"category"`
	Gain     float64 `json:"gain"`
	Loop     bool    `json:"loop"`
}

// broadcastReq is the body for POST /api/broadcast.
type broadcastReq struct {
	Text     string  `json:"text"`
	Preset   string  `json:"preset"`
	Category string  `json:"category"`
	Voice    string  `json:"voice"`
	Gain     float64 `json:"gain"`
	Loop     bool    `json:"loop"`
}

// genPresetReq is the body for POST /api/library.
type genPresetReq struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Text     string `json:"text"`
	Voice    string `json:"voice"`
}

// Speak handles POST /api/speak — TTS → camera.
func (h *Handlers) Speak(c echo.Context) error {
	log := h.logger(c)

	var req speakReq
	err := c.Bind(&req)
	if err != nil || req.Camera == "" || req.Text == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "camera and text required")
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

	timings, err := h.speakText(log, req.Camera, req.Text, req.Voice, req.Gain)
	if err != nil {
		log.Error("speak: failed", "camera", req.Camera, "elapsed", time.Since(start), "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	log.Info("speak: done", "camera", req.Camera, "elapsed", time.Since(start), "ttfs_ms", timings.TTFS())
	return c.JSON(http.StatusOK, map[string]any{
		"status":   "ok",
		"timings":  timings.Ms(),
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

	timings, err := h.playPreset(log, req.Camera, req.Category, req.Preset, req.Gain, req.Loop)
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

// PlayURL handles POST /api/play-url — download URL → transcode → camera.
func (h *Handlers) PlayURL(c echo.Context) error {
	log := h.logger(c)

	var req struct {
		Camera string  `json:"camera"`
		URL    string  `json:"url"`
		Gain   float64 `json:"gain"`
	}
	if err := c.Bind(&req); err != nil || req.Camera == "" || req.URL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "camera and url required")
	}

	if req.Gain <= 0 {
		req.Gain = 3.0
	}

	// Validate URL scheme to prevent SSRF (only http/https allowed), and
	// derive a redacted URL for logging/event storage.
	parsedURL, err := neturl.Parse(req.URL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return echo.NewHTTPError(http.StatusBadRequest, "url must be http or https")
	}

	redactedURL := util.RedactURL(parsedURL)

	log.Info("play-url: request", "camera", req.Camera, "url", redactedURL, "gain", req.Gain)
	start := time.Now()

	cam, err := h.reg.Get(req.Camera)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	resp, err := http.Get(req.URL)
	if err != nil {
		log.Error("play-url: download failed", "camera", req.Camera, "url", redactedURL, "err", err)
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("download failed: %s", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Error(
			"play-url: download bad status",
			"camera",
			req.Camera,
			"url",
			redactedURL,
			"status",
			resp.StatusCode,
		)
		return echo.NewHTTPError(
			http.StatusBadGateway,
			fmt.Sprintf("download returned HTTP %d", resp.StatusCode),
		)
	}

	tmp, err := os.CreateTemp(h.tmpDir, "camspeak_url_*")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return echo.NewHTTPError(
			http.StatusInternalServerError,
			fmt.Sprintf("saving download: %s", err),
		)
	}
	if err := tmp.Close(); err != nil {
		return echo.NewHTTPError(
			http.StatusInternalServerError,
			fmt.Sprintf("closing temp file: %s", err),
		)
	}

	// Transcode to raw
	raw, err := os.CreateTemp(h.tmpDir, "camspeak_url_*.raw")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	rawName := raw.Name()
	raw.Close()

	if err := transcodeFileToRawGainWithPrime(tmpName, rawName, req.Gain, h.cfg.PrimeSilenceMs); err != nil {
		os.Remove(rawName)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	defer os.Remove(rawName)

	log.Debug("play-url: sending to camera", "camera", req.Camera, "url", redactedURL)
	setPlayback(req.Camera, "play-url", redactedURL)
	if _, err := cam.SendRaw(rawName); err != nil {
		clearPlayback(req.Camera)
		log.Error(
			"play-url: send failed",
			"camera",
			req.Camera,
			"elapsed",
			time.Since(start),
			"err",
			err,
		)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	log.Info(
		"play-url: done",
		"camera",
		req.Camera,
		"url",
		redactedURL,
		"elapsed",
		time.Since(start),
	)
	h.events.publish(
		event{Camera: req.Camera, Action: "play-url", Text: redactedURL, At: time.Now()},
	)
	clearPlayback(req.Camera)

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
	// Body is optional — ignore bind errors (empty body = stop all)
	_ = c.Bind(&req)

	if req.Camera != "" {
		if err := h.reg.Stop(req.Camera); err != nil {
			log.Warn("stop: camera not found", "camera", req.Camera, "err", err)
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		stopStream(req.Camera)
		clearPlayback(req.Camera)
		h.resetAirPlay(req.Camera, log)
		log.Info("stop: stopped and reset camera", "camera", req.Camera)
		h.events.publish(event{Camera: req.Camera, Action: "stop", At: time.Now()})
		return c.JSON(http.StatusOK, map[string]string{"status": "stopped", "camera": req.Camera})
	}

	// Stop all cameras, live streams, and reset AirPlay receivers.
	h.reg.StopAll()
	stopAllStreams()
	clearAllPlayback()
	h.resetAllAirPlay(log)
	log.Info("stop: stopped and reset all cameras")
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
	_ = c.Bind(&req)

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
	_ = c.Bind(&req)

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
		return
	}
	h.airplayMgr.Disable(name)
	if err := h.airplayMgr.Enable(name); err != nil {
		log.Warn("stop: AirPlay reset failed", "camera", name, "err", err)
	}
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
		h.airplayMgr.Disable(name)
		if err := h.airplayMgr.Enable(name); err != nil {
			log.Warn("stop: AirPlay reset failed", "camera", name, "err", err)
		}
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

	cam, err := h.reg.Get(req.Camera)
	if err != nil {
		log.Warn("beep: camera not found", "camera", req.Camera, "err", err)
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	raw, err := GenerateBeep(h.tmpDir)
	if err != nil {
		log.Error("beep: generating tone failed", "camera", req.Camera, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer os.Remove(raw)

	log.Info("beep: sending", "camera", req.Camera, "type", h.cfg.Cameras[req.Camera].Type)
	start := time.Now()

	setPlayback(req.Camera, "beep", "800Hz test tone")
	if _, err := cam.SendRaw(raw); err != nil {
		clearPlayback(req.Camera)
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
	clearPlayback(req.Camera)

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// Cameras handles GET /api/cameras — returns only enabled cameras.
func (h *Handlers) Cameras(c echo.Context) error {
	status := h.reg.Status()

	out := make([]map[string]any, 0)
	for name, cfg := range h.cfg.Cameras {
		if !cfg.Enabled {
			continue
		}
		out = append(out, map[string]any{
			"name":            name,
			"type":            cfg.Type,
			"ip":              cfg.IP,
			"online":          status[name],
			"vision_prompt":   cfg.VisionPrompt,
			"vision_stream":   cfg.VisionStream,
			"vision_width":    cfg.VisionWidth,
			"note":            cfg.Note,
			"airplay_enabled": cfg.AirPlayEnabled,
			"airplay_name":    cfg.AirPlayName,
			"airplay_model":   cfg.AirPlayModel,
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
	names := make([]string, 0, len(h.cfg.Cameras))
	for name, cfg := range h.cfg.Cameras {
		if cfg.Enabled {
			names = append(names, name)
		}
	}
	return c.JSON(http.StatusOK, getAllPlayback(names))
}

// Voices handles GET /api/voices.
func (h *Handlers) Voices(c echo.Context) error {
	return c.JSON(http.StatusOK, h.tts.Voices())
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

	ch := h.events.subscribe()
	defer h.events.unsubscribe(ch)

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

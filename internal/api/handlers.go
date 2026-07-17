package api

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"sync"
	"time"

	clog "github.com/charmbracelet/log"
	"github.com/labstack/echo/v4"

	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/library"
	"github.com/jeeftor/camspeak/internal/tts"
	"github.com/jeeftor/camspeak/internal/vision"
)

// Handlers holds all route handler dependencies.
type Handlers struct {
	cfg    *config.Config
	reg    *cameras.Registry
	store  *library.Store
	tts    *tts.Client
	vision *vision.Client
	events *eventBus
	db     *sql.DB
	tmpDir string
	log    *clog.Logger
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
}

// broadcastReq is the body for POST /api/broadcast.
type broadcastReq struct {
	Text     string  `json:"text"`
	Preset   string  `json:"preset"`
	Category string  `json:"category"`
	Voice    string  `json:"voice"`
	Gain     float64 `json:"gain"`
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
	var req speakReq
	err := c.Bind(&req)
	if err != nil || req.Camera == "" || req.Text == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "camera and text required")
	}

	h.log.Info("speak: request", "camera", req.Camera, "text_len", len(req.Text), "voice", req.Voice, "gain", req.Gain)
	start := time.Now()

	err = h.speakText(req.Camera, req.Text, req.Voice, req.Gain)
	if err != nil {
		h.log.Error("speak: failed", "camera", req.Camera, "elapsed", time.Since(start), "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	h.log.Info("speak: done", "camera", req.Camera, "elapsed", time.Since(start))
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// Play handles POST /api/play — preset → camera.
func (h *Handlers) Play(c echo.Context) error {
	var req playReq
	err := c.Bind(&req)
	if err != nil || req.Camera == "" || req.Preset == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "camera and preset required")
	}

	h.log.Info("play: request", "camera", req.Camera, "preset", req.Preset, "category", req.Category, "gain", req.Gain)
	start := time.Now()

	err = h.playPreset(req.Camera, req.Category, req.Preset, req.Gain)
	if err != nil {
		h.log.Error("play: failed", "camera", req.Camera, "preset", req.Preset, "elapsed", time.Since(start), "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	h.log.Info("play: done", "camera", req.Camera, "preset", req.Preset, "elapsed", time.Since(start))
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// PlayURL handles POST /api/play-url — download URL → transcode → camera.
func (h *Handlers) PlayURL(c echo.Context) error {
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

	h.log.Info("play-url: request", "camera", req.Camera, "url", req.URL, "gain", req.Gain)
	start := time.Now()

	cam, err := h.reg.Get(req.Camera)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	// Download to temp file
	resp, err := http.Get(req.URL)
	if err != nil {
		h.log.Error("play-url: download failed", "camera", req.Camera, "url", req.URL, "err", err)
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("download failed: %s", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		h.log.Error("play-url: download bad status", "camera", req.Camera, "url", req.URL, "status", resp.StatusCode)
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("download returned HTTP %d", resp.StatusCode))
	}

	tmp, err := os.CreateTemp(h.tmpDir, "camspeak_url_*")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer os.Remove(tmp.Name())

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("saving download: %s", err))
	}
	tmp.Close()

	// Transcode to raw
	raw, err := os.CreateTemp(h.tmpDir, "camspeak_url_*.raw")
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	rawName := raw.Name()
	raw.Close()

	if err := transcodeFileToRawGain(tmp.Name(), rawName, req.Gain); err != nil {
		os.Remove(rawName)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	defer os.Remove(rawName)

	h.log.Debug("play-url: sending to camera", "camera", req.Camera, "url", req.URL)
	if err := cam.SendRaw(rawName); err != nil {
		h.log.Error("play-url: send failed", "camera", req.Camera, "elapsed", time.Since(start), "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	h.log.Info("play-url: done", "camera", req.Camera, "url", req.URL, "elapsed", time.Since(start))
	h.events.publish(event{Camera: req.Camera, Action: "play-url", Text: req.URL, At: time.Now()})

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// Beep handles POST /api/beep — 800Hz test tone → camera.
func (h *Handlers) Beep(c echo.Context) error {
	var req struct {
		Camera string `json:"camera"`
	}
	if err := c.Bind(&req); err != nil || req.Camera == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "camera required")
	}

	cam, err := h.reg.Get(req.Camera)
	if err != nil {
		h.log.Warn("beep: camera not found", "camera", req.Camera, "err", err)
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	raw, err := GenerateBeep(h.tmpDir)
	if err != nil {
		h.log.Error("beep: generating tone failed", "camera", req.Camera, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer os.Remove(raw)

	h.log.Info("beep: sending", "camera", req.Camera, "type", h.cfg.Cameras[req.Camera].Type)
	start := time.Now()

	if err := cam.SendRaw(raw); err != nil {
		h.log.Error("beep: send failed", "camera", req.Camera, "elapsed", time.Since(start), "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	h.log.Info("beep: sent", "camera", req.Camera, "elapsed", time.Since(start))
	h.events.publish(event{Camera: req.Camera, Action: "beep", At: time.Now()})

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// Snapshot handles GET /api/snapshot/:camera — proxies Frigate snapshot as JPEG.
func (h *Handlers) Snapshot(c echo.Context) error {
	camera := c.Param("camera")
	if camera == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "camera required")
	}
	if h.cfg.FrigateURL == "" {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "frigate URL not configured")
	}

	snapURL := fmt.Sprintf("%s/api/%s/latest.jpg", h.cfg.FrigateURL, camera)
	resp, err := http.Get(snapURL)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("frigate returned HTTP %d", resp.StatusCode))
	}

	c.Response().Header().Set("Content-Type", "image/jpeg")
	c.Response().Header().Set("Cache-Control", "no-cache")
	return c.Stream(http.StatusOK, "image/jpeg", resp.Body)
}

// Describe handles POST /api/describe — Frigate snapshot → vision model → TTS → camera.
func (h *Handlers) Describe(c echo.Context) error {
	var req struct {
		Camera string  `json:"camera"`
		Prompt string  `json:"prompt"`
		Gain   float64 `json:"gain"`
	}
	if err := c.Bind(&req); err != nil || req.Camera == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "camera required")
	}

	if h.cfg.FrigateURL == "" {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "frigate URL not configured")
	}

	if h.vision == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "vision model not configured")
	}

	start := time.Now()
	h.log.Info("describe: request", "camera", req.Camera)

	// 1. Fetch snapshot from Frigate
	snapURL := fmt.Sprintf("%s/api/%s/latest.jpg", h.cfg.FrigateURL, req.Camera)
	snapStart := time.Now()
	snapResp, err := http.Get(snapURL)
	if err != nil {
		h.log.Error("describe: snapshot failed", "camera", req.Camera, "err", err)
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("frigate snapshot: %s", err))
	}
	defer snapResp.Body.Close()

	if snapResp.StatusCode != 200 {
		h.log.Error("describe: snapshot bad status", "camera", req.Camera, "status", snapResp.StatusCode)
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("frigate returned HTTP %d", snapResp.StatusCode))
	}

	imageBytes, err := io.ReadAll(snapResp.Body)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("reading snapshot: %s", err))
	}
	h.log.Debug("describe: snapshot fetched", "camera", req.Camera, "bytes", len(imageBytes), "elapsed", time.Since(snapStart))

	// 2. Send to vision model
	visionStart := time.Now()
	description, err := h.vision.Describe(imageBytes, "image/jpeg", req.Prompt)
	if err != nil {
		h.log.Error("describe: vision failed", "camera", req.Camera, "err", err)
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("vision: %s", err))
	}
	h.log.Info("describe: vision result", "camera", req.Camera, "text", description, "elapsed", time.Since(visionStart))

	// 3. TTS
	voice := h.cfg.TTS.DefaultVoice
	ttsStart := time.Now()
	wav, err := h.tts.Speak(description, voice)
	if err != nil {
		h.log.Error("describe: TTS failed", "camera", req.Camera, "err", err)
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("TTS: %s", err))
	}
	h.log.Debug("describe: TTS generated", "camera", req.Camera, "wav_bytes", len(wav), "elapsed", time.Since(ttsStart))

	// 4. Transcode + send to camera
	gain := req.Gain
	if gain <= 0 {
		gain = 3.0
	}

	rawPath, err := wavBytesToRaw(wav, h.tmpDir, gain)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("transcoding: %s", err))
	}
	defer os.Remove(rawPath)

	cam, err := h.reg.Get(req.Camera)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	sendStart := time.Now()
	if err := cam.SendRaw(rawPath); err != nil {
		h.log.Error("describe: send failed", "camera", req.Camera, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.log.Debug("describe: camera send complete", "camera", req.Camera, "elapsed", time.Since(sendStart))

	h.log.Info("describe: done", "camera", req.Camera, "elapsed", time.Since(start))
	h.events.publish(event{Camera: req.Camera, Action: "describe", Text: description, At: time.Now()})

	snapB64 := base64.StdEncoding.EncodeToString(imageBytes)
	return c.JSON(http.StatusOK, map[string]string{
		"status":      "ok",
		"description": description,
		"image":       "data:image/jpeg;base64," + snapB64,
	})
}

// Broadcast handles POST /api/broadcast — TTS or preset → all cameras in parallel.
func (h *Handlers) Broadcast(c echo.Context) error {
	var req broadcastReq
	err := c.Bind(&req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}

	if req.Text == "" && req.Preset == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "text or preset required")
	}

	names := h.reg.Names()
	mode := "tts"
	if req.Preset != "" {
		mode = "preset"
	}
	h.log.Info("broadcast: starting", "mode", mode, "cameras", names, "text_len", len(req.Text))
	start := time.Now()

	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)

	errs := make([]string, 0)
	succeeded := make([]string, 0)

	for _, name := range names {
		wg.Add(1)
		go func(cam string) {
			defer wg.Done()

			camStart := time.Now()
			var err error
			if req.Preset != "" {
				err = h.playPreset(cam, req.Category, req.Preset, req.Gain)
			} else {
				err = h.speakText(cam, req.Text, req.Voice, req.Gain)
			}

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				h.log.Error("broadcast: camera failed", "camera", cam, "elapsed", time.Since(camStart), "err", err)
				errs = append(errs, fmt.Sprintf("%s: %s", cam, err))
			} else {
				h.log.Info("broadcast: camera done", "camera", cam, "elapsed", time.Since(camStart))
				succeeded = append(succeeded, cam)
			}
		}(name)
	}

	wg.Wait()

	h.log.Info("broadcast: complete", "succeeded", len(succeeded), "failed", len(errs), "elapsed", time.Since(start))

	if len(errs) > 0 {
		return c.JSON(http.StatusMultiStatus, map[string]any{
			"succeeded": succeeded,
			"errors":    errs,
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status":    "ok",
		"succeeded": succeeded,
	})
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
			"name":   name,
			"type":   cfg.Type,
			"ip":     cfg.IP,
			"online": status[name],
		})
	}

	return c.JSON(http.StatusOK, out)
}

// Voices handles GET /api/voices.
func (h *Handlers) Voices(c echo.Context) error {
	return c.JSON(http.StatusOK, h.tts.Voices())
}

// ListLibrary handles GET /api/library.
func (h *Handlers) ListLibrary(c echo.Context) error {
	presets, err := h.store.List()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Filter out transient _tmp presets (created by ad-hoc speak, raw files
	// are deleted immediately but DB rows linger).
	filtered := presets[:0]
	for _, p := range presets {
		if p.Category != "_tmp" {
			filtered = append(filtered, p)
		}
	}

	return c.JSON(http.StatusOK, filtered)
}

// TTSPreview handles POST /api/tts/preview — generates TTS and returns WAV audio.
func (h *Handlers) TTSPreview(c echo.Context) error {
	var req struct {
		Text  string `json:"text"`
		Voice string `json:"voice"`
	}
	if err := c.Bind(&req); err != nil || req.Text == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "text required")
	}

	voice := req.Voice
	if voice == "" {
		voice = h.cfg.TTS.DefaultVoice
	}

	wav, err := h.tts.Speak(req.Text, voice)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("TTS failed: %s", err))
	}

	return c.Blob(http.StatusOK, "audio/wav", wav)
}

// GeneratePreset handles POST /api/library — TTS → save preset.
func (h *Handlers) GeneratePreset(c echo.Context) error {
	var req genPresetReq
	if err := c.Bind(&req); err != nil || req.Name == "" || req.Text == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name and text required")
	}

	if req.Category == "" {
		req.Category = "default"
	}

	voice := req.Voice
	if voice == "" {
		voice = h.cfg.TTS.DefaultVoice
	}

	wav, err := h.tts.Speak(req.Text, voice)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("TTS failed: %s", err))
	}

	preset, err := h.store.Save(req.Category, req.Name, req.Text, voice, wav)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, preset)
}

// UploadPreset handles POST /api/library/upload — audio file → save preset.
func (h *Handlers) UploadPreset(c echo.Context) error {
	name := c.FormValue("name")
	category := c.FormValue("category")

	if name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name required")
	}

	if category == "" {
		category = "uploads"
	}

	file, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "file required")
	}

	src, err := file.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer src.Close()

	// Sanitize filename for temp file pattern (strip path separators, wildcards)
	safeName := sanitizeFilename(file.Filename)

	tmp, err := os.CreateTemp(h.tmpDir, "camspeak_upload_*_"+safeName)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer os.Remove(tmp.Name())

	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()

		return echo.NewHTTPError(http.StatusInternalServerError, fmt.Sprintf("reading upload: %s", err))
	}

	tmp.Close()

	preset, err := h.store.SaveFile(category, name, tmp.Name())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, preset)
}

// DeletePreset handles DELETE /api/library/:category/:name.
func (h *Handlers) DeletePreset(c echo.Context) error {
	err := h.store.Delete(c.Param("category"), c.Param("name"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

// PreviewPreset handles GET /api/library/:category/:name/preview — streams WAV.
func (h *Handlers) PreviewPreset(c echo.Context) error {
	preset, err := h.store.Get(c.Param("category"), c.Param("name"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	// Convert raw → WAV on the fly for browser preview
	wav, err := rawToWAV(preset.RawPath, h.tmpDir)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer os.Remove(wav)

	return c.File(wav)
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
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().WriteHeader(http.StatusOK)

	// Send recent history on connect
	if recent, err := h.events.recentEvents(50); err == nil {
		for _, v := range slices.Backward(recent) {
			data, _ := json.Marshal(v)
			fmt.Fprintf(c.Response(), "data: %s\n\n", data)
		}

		c.Response().Flush()
	}

	ch := h.events.subscribe()
	defer h.events.unsubscribe(ch)

	for {
		select {
		case ev := <-ch:
			data, _ := json.Marshal(ev)
			fmt.Fprintf(c.Response(), "data: %s\n\n", data)
			c.Response().Flush()
		case <-c.Request().Context().Done():
			return nil
		}
	}
}

// --- Internal helpers ---

func (h *Handlers) speakText(cameraName, text, voice string, gain float64) error {
	cam, err := h.reg.Get(cameraName)
	if err != nil {
		return err
	}

	if voice == "" {
		voice = h.cfg.TTS.DefaultVoice
	}

	if gain <= 0 {
		gain = 3.0 // default boost
	}

	ttsStart := time.Now()
	wav, err := h.tts.Speak(text, voice)
	if err != nil {
		return fmt.Errorf("TTS: %w", err)
	}
	h.log.Debug("speak: TTS generated", "camera", cameraName, "voice", voice, "wav_bytes", len(wav), "elapsed", time.Since(ttsStart))

	rawPath, err := wavBytesToRaw(wav, h.tmpDir, gain)
	if err != nil {
		return fmt.Errorf("transcoding: %w", err)
	}
	defer os.Remove(rawPath)

	h.log.Debug("speak: sending to camera", "camera", cameraName)
	sendStart := time.Now()
	if err := cam.SendRaw(rawPath); err != nil {
		return fmt.Errorf("sending to camera: %w", err)
	}
	h.log.Debug("speak: camera send complete", "camera", cameraName, "elapsed", time.Since(sendStart))

	h.events.publish(event{Camera: cameraName, Action: "speak", Text: text, At: time.Now()})

	return nil
}

func (h *Handlers) playPreset(cameraName, category, presetName string, gain float64) error {
	cam, err := h.reg.Get(cameraName)
	if err != nil {
		return err
	}

	var preset *library.Preset
	if category != "" {
		preset, err = h.store.Get(category, presetName)
	} else {
		preset, err = h.store.GetByName(presetName)
	}

	if err != nil {
		return err
	}

	// If gain is specified, re-transcode the raw file with the gain filter.
	// The stored raw is already G.711ulaw 8kHz, so we read it as mulaw and
	// apply volume, then output mulaw again.
	sendPath := preset.RawPath
	if gain > 0 && gain != 3.0 {
		boosted, err := boostRawGain(preset.RawPath, h.tmpDir, gain)
		if err != nil {
			h.log.Warn("play: gain boost failed, sending original", "err", err)
		} else {
			defer os.Remove(boosted)
			sendPath = boosted
		}
	}

	h.log.Debug("play: sending preset", "camera", cameraName, "preset", preset.Name, "raw_bytes", preset.Size, "gain", gain)
	sendStart := time.Now()
	if err := cam.SendRaw(sendPath); err != nil {
		return fmt.Errorf("sending to camera: %w", err)
	}
	h.log.Debug("play: camera send complete", "camera", cameraName, "elapsed", time.Since(sendStart))

	h.events.publish(event{Camera: cameraName, Action: "play", Text: preset.Name, At: time.Now()})

	return nil
}

// SpeakForMQTT is called by the MQTT subscriber.
func (h *Handlers) SpeakForMQTT(cams []string, text, preset, voice string) {
	var wg sync.WaitGroup
	for _, cam := range cams {
		wg.Add(1)
		go func(c string) {
			defer wg.Done()

			if preset != "" {
				h.playPreset(c, "", preset, 3.0) //nolint:errcheck
			} else if text != "" {
				h.speakText(c, text, voice, 3.0) //nolint:errcheck
			}
		}(cam)
	}

	wg.Wait()
}

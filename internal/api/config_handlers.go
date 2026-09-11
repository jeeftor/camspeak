package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/util"
	"github.com/jeeftor/camspeak/internal/vision"
)

// GetConfig handles GET /api/config — returns the current runtime config.
func (h *Handlers) GetConfig(c echo.Context) error {
	h.cfgMu.Lock()
	cfg := h.cfg.Sanitized()
	h.cfgMu.Unlock()
	return c.JSON(http.StatusOK, cfg)
}

// GetVisionConfig handles GET /api/config/vision — returns vision config.
func (h *Handlers) GetVisionConfig(c echo.Context) error {
	h.cfgMu.Lock()
	cfg := h.cfg.Vision.Sanitized()
	h.cfgMu.Unlock()
	return c.JSON(http.StatusOK, cfg)
}

// UpdateVisionConfig handles PUT /api/config/vision — updates vision config.
func (h *Handlers) UpdateVisionConfig(c echo.Context) error {
	h.configEditMu.Lock()
	defer h.configEditMu.Unlock()
	var req struct {
		config.VisionConfig
		ClearAPIKey bool `json:"clear_api_key"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	if req.APIKey == "" && !req.ClearAPIKey {
		if err := h.db.QueryRow(`SELECT value FROM preferences WHERE key = 'vision_api_key'`).Scan(&req.APIKey); err != nil {
			req.APIKey = ""
		}
	}

	prefs := map[string]string{
		"vision_url":     req.URL,
		"vision_model":   req.Model,
		"vision_api_key": req.APIKey,
		"vision_prompt":  req.Prompt,
	}
	if err := config.SetPreferences(h.db, prefs); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	h.cfgMu.Lock()
	h.cfg.Vision = req.VisionConfig
	config.ApplyEnvOverrides(h.cfg)
	h.vision = vision.NewClient(h.cfg.Vision.URL, h.cfg.Vision.Model, h.cfg.Vision.APIKey)
	effective := h.cfg.Vision.Sanitized()
	h.cfgMu.Unlock()

	h.logger(c).Info(
		"vision config updated",
		"url", util.RedactURLString(req.URL),
		"model", req.Model,
		"has_prompt", req.Prompt != "",
	)
	return c.JSON(http.StatusOK, effective)
}

// TestVisionConfig handles POST /api/config/vision/test — probes the vision endpoint from the server.
func (h *Handlers) TestVisionConfig(c echo.Context) error {
	var req struct {
		URL    string `json:"url"`
		APIKey string `json:"api_key"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	current := h.configSnapshot().Vision
	if req.APIKey == "" && req.URL == current.URL {
		req.APIKey = current.APIKey
	}
	// Derive models endpoint from the chat completions URL.
	// e.g. http://host/v1/chat/completions → http://host/v1/models
	base := req.URL
	if idx := strings.Index(base, "/v1/"); idx >= 0 {
		base = base[:idx+4] + "models"
	} else {
		base = strings.TrimRight(base, "/") + "/v1/models"
	}
	h.logger(c).Info("testing vision endpoint", "url", util.RedactURLString(base))

	httpReq, err := http.NewRequestWithContext(c.Request().Context(), http.MethodGet, base, nil)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{"ok": false, "message": err.Error()})
	}
	if req.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(httpReq)
	if err != nil {
		h.logger(c).Warn("vision endpoint test failed", "url", base, "err", err)
		return c.JSON(http.StatusOK, map[string]interface{}{"ok": false, "message": err.Error()})
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&data)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		h.logger(c).Warn("vision endpoint HTTP error", "url", base, "status", resp.StatusCode)
		return c.JSON(
			http.StatusOK,
			map[string]interface{}{"ok": false, "message": fmt.Sprintf("HTTP %d", resp.StatusCode)},
		)
	}

	// Count models if response has "data" array.
	count := 0
	if d, ok := data["data"].([]interface{}); ok {
		count = len(d)
	}
	h.logger(c).Info("vision endpoint test ok", "url", base, "models", count)
	return c.JSON(http.StatusOK, map[string]interface{}{"ok": true, "models": count, "data": data})
}

// TestTTSConfig handles POST /api/config/tts/test — probes a TTS endpoint from the server.
// Accepts {url, api_key, model} and checks the model catalog without generating audio.
func (h *Handlers) TestTTSConfig(c echo.Context) error {
	var req struct {
		URL    string `json:"url"`
		APIKey string `json:"api_key"`
		Model  string `json:"model"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	if req.APIKey == "" {
		current := h.configSnapshot().TTS
		if req.URL == current.URL {
			req.APIKey = current.APIKey
		} else if presets, err := config.ListTTSPresets(h.db); err == nil {
			for _, preset := range presets {
				if preset.Endpoint == req.URL {
					req.APIKey = preset.APIKey
					break
				}
			}
		}
	}
	// Derive base URL — strip any path after the host:port so we can probe /v1/models.
	base := req.URL
	if idx := strings.Index(base, "/v1/"); idx >= 0 {
		base = base[:idx+4] + "models"
	} else {
		base = strings.TrimRight(base, "/") + "/v1/models"
	}
	h.logger(c).Info("testing TTS endpoint", "url", util.RedactURLString(base))

	httpReq, err := http.NewRequestWithContext(c.Request().Context(), http.MethodGet, base, nil)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{"ok": false, "message": err.Error()})
	}
	if req.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(httpReq)
	if err != nil {
		return c.JSON(http.StatusOK, map[string]interface{}{"ok": false, "message": err.Error()})
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.JSON(
			http.StatusOK,
			map[string]interface{}{"ok": false, "message": fmt.Sprintf("HTTP %d", resp.StatusCode)},
		)
	}
	var catalog struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&catalog); err != nil ||
		catalog.Data == nil {
		return c.JSON(
			http.StatusOK,
			map[string]interface{}{
				"ok":      false,
				"message": "Endpoint responded, but did not return a valid model catalog",
			},
		)
	}
	models := make([]string, 0, len(catalog.Data))
	found := false
	for _, model := range catalog.Data {
		if model.ID == "" {
			continue
		}
		models = append(models, model.ID)
		found = found || model.ID == req.Model
	}
	message := "Endpoint reachable; model catalog received. Speech generation has not been tested."
	ok := true
	if req.Model != "" {
		if found {
			message = "Model is listed. Speech generation has not been tested."
		} else {
			ok = false
			message = fmt.Sprintf("Model %q is not listed by this endpoint. Choose an exact server model ID; this does not mean the model is merely unloaded.", req.Model)
		}
	}
	return c.JSON(
		http.StatusOK,
		map[string]interface{}{"ok": ok, "message": message, "models": models},
	)
}

// GetSettings handles GET /api/config/settings — returns general settings.
func (h *Handlers) GetSettings(c echo.Context) error {
	cfg := h.configSnapshot().Sanitized()
	return c.JSON(http.StatusOK, map[string]interface{}{
		"frigate_url":  cfg.FrigateURL,
		"go2rtc_url":   cfg.Go2rtcURL,
		"advertise_ip": cfg.AdvertiseIP,
	})
}

// UpdateSettings handles PUT /api/config/settings — saves general settings.
func (h *Handlers) UpdateSettings(c echo.Context) error {
	h.configEditMu.Lock()
	defer h.configEditMu.Unlock()
	var req struct {
		FrigateURL  string `json:"frigate_url"`
		Go2rtcURL   string `json:"go2rtc_url"`
		AdvertiseIP string `json:"advertise_ip"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	cfg := h.configSnapshot()
	cfg.FrigateURL, cfg.Go2rtcURL, cfg.AdvertiseIP = req.FrigateURL, req.Go2rtcURL, req.AdvertiseIP
	config.ApplyEnvOverrides(&cfg)
	go2rtcURL := cfg.Go2rtcURL
	if go2rtcURL == "" {
		go2rtcURL = cameras.FindGo2rtcURL(cfg.FrigateURL)
	}
	if err := h.reg.ValidateRouting(go2rtcURL); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := config.SetPreferences(h.db, map[string]string{
		"frigate_url": req.FrigateURL, "go2rtc_url": req.Go2rtcURL, "advertise_ip": req.AdvertiseIP,
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	previousURL, previousIP := h.reg.Routing()
	if previousURL != go2rtcURL || previousIP != cfg.AdvertiseIP {
		// SetRouting replaces every enabled speaker. Retire their supervisors first
		// so prepared audio and reconnect loops cannot reopen an obsolete client.
		for _, name := range h.reg.Names() {
			stopOperation(name)
			stopStream(name)
			clearPlayback(name)
		}
	}
	if err := h.reg.SetRouting(go2rtcURL, cfg.AdvertiseIP); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	h.cfgMu.Lock()
	h.cfg.FrigateURL = cfg.FrigateURL
	h.cfg.Go2rtcURL = cfg.Go2rtcURL
	h.cfg.AdvertiseIP = cfg.AdvertiseIP
	h.cfgMu.Unlock()
	if h.airplayMgr != nil {
		if err := h.airplayMgr.UpdateRouting(cfg.AdvertiseIP); err != nil {
			h.logger(c).Warn("AirPlay address update failed", "err", err)
		}
	}
	h.logger(c).Info("settings updated",
		"frigate_url", util.RedactURLString(req.FrigateURL),
		"go2rtc_url", util.RedactURLString(req.Go2rtcURL),
		"advertise_ip", req.AdvertiseIP,
	)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"frigate_url":  util.RedactURLString(cfg.FrigateURL),
		"go2rtc_url":   util.RedactURLString(cfg.Go2rtcURL),
		"advertise_ip": cfg.AdvertiseIP,
	})
}

// TestSettingsURL handles POST /api/config/settings/test — probes a Frigate or go2rtc URL
// from the server side so the request appears in logs and avoids browser CORS issues.
func (h *Handlers) TestSettingsURL(c echo.Context) error {
	var req struct {
		Type string `json:"type"` // "frigate" or "go2rtc"
		URL  string `json:"url"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	if req.URL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "url is required")
	}

	base := strings.TrimRight(req.URL, "/")
	// Each service has a different health/info endpoint.
	var target string
	switch req.Type {
	case "go2rtc":
		target = base + "/api/streams"
	default: // "frigate"
		target = base + "/api/"
	}
	h.logger(c).Info("testing settings URL", "type", req.Type, "url", target)

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Get(target) //nolint:noctx
	if err != nil {
		h.logger(c).Warn("settings URL test failed", "type", req.Type, "url", target, "err", err)
		return c.JSON(http.StatusOK, map[string]interface{}{"ok": false, "message": err.Error()})
	}
	defer resp.Body.Close()

	var data map[string]interface{}
	_ = json.NewDecoder(resp.Body).Decode(&data)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		h.logger(c).
			Warn("settings URL test HTTP error", "type", req.Type, "status", resp.StatusCode)
		return c.JSON(
			http.StatusOK,
			map[string]interface{}{"ok": false, "message": fmt.Sprintf("HTTP %d", resp.StatusCode)},
		)
	}

	h.logger(c).
		Info("settings URL test ok", "type", req.Type, "url", target, "status", resp.StatusCode)
	return c.JSON(http.StatusOK, map[string]interface{}{"ok": true, "data": data})
}

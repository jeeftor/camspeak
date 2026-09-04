package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

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
	var req config.VisionConfig
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}

	prefs := map[string]string{
		"vision_url":     req.URL,
		"vision_model":   req.Model,
		"vision_api_key": req.APIKey,
		"vision_prompt":  req.Prompt,
	}
	for key, val := range prefs {
		if err := config.SetPreference(h.db, key, val); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}

	h.cfgMu.Lock()
	h.cfg.Vision = req
	h.vision = vision.NewClient(req.URL, req.Model, req.APIKey)
	h.cfgMu.Unlock()

	h.logger(c).Info(
		"vision config updated",
		"url", util.RedactURLString(req.URL),
		"model", req.Model,
		"has_prompt", req.Prompt != "",
	)
	return c.JSON(http.StatusOK, req.Sanitized())
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
	// Derive models endpoint from the chat completions URL.
	// e.g. http://host/v1/chat/completions → http://host/v1/models
	base := req.URL
	if idx := strings.Index(base, "/v1/"); idx >= 0 {
		base = base[:idx+4] + "models"
	} else {
		base = strings.TrimRight(base, "/") + "/v1/models"
	}
	h.logger(c).Info("testing vision endpoint", "url", base)

	httpReq, err := http.NewRequest(http.MethodGet, base, nil)
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
// Accepts {url, api_key} and tries GET {base}/v1/models to verify reachability.
func (h *Handlers) TestTTSConfig(c echo.Context) error {
	var req struct {
		URL    string `json:"url"`
		APIKey string `json:"api_key"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	// Derive base URL — strip any path after the host:port so we can probe /v1/models.
	base := req.URL
	if idx := strings.Index(base, "/v1/"); idx >= 0 {
		base = base[:idx+4] + "models"
	} else {
		base = strings.TrimRight(base, "/") + "/v1/models"
	}
	h.logger(c).Info("testing TTS endpoint", "url", base)

	httpReq, err := http.NewRequest(http.MethodGet, base, nil)
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
	return c.JSON(http.StatusOK, map[string]interface{}{"ok": true, "message": "Connected"})
}

// GetSettings handles GET /api/config/settings — returns general settings.
func (h *Handlers) GetSettings(c echo.Context) error {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	return c.JSON(http.StatusOK, map[string]interface{}{
		"frigate_url":  h.cfg.FrigateURL,
		"go2rtc_url":   h.cfg.Go2rtcURL,
		"advertise_ip": h.cfg.AdvertiseIP,
	})
}

// UpdateSettings handles PUT /api/config/settings — saves general settings.
func (h *Handlers) UpdateSettings(c echo.Context) error {
	var req struct {
		FrigateURL  string `json:"frigate_url"`
		Go2rtcURL   string `json:"go2rtc_url"`
		AdvertiseIP string `json:"advertise_ip"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	if err := config.SetPreference(h.db, "frigate_url", req.FrigateURL); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := config.SetPreference(h.db, "go2rtc_url", req.Go2rtcURL); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := config.SetPreference(h.db, "advertise_ip", req.AdvertiseIP); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.cfgMu.Lock()
	h.cfg.FrigateURL = req.FrigateURL
	h.cfg.Go2rtcURL = req.Go2rtcURL
	h.cfg.AdvertiseIP = req.AdvertiseIP
	h.cfgMu.Unlock()
	h.logger(c).Info("settings updated",
		"frigate_url", req.FrigateURL,
		"go2rtc_url", req.Go2rtcURL,
		"advertise_ip", req.AdvertiseIP,
	)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"frigate_url":  req.FrigateURL,
		"go2rtc_url":   req.Go2rtcURL,
		"advertise_ip": req.AdvertiseIP,
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

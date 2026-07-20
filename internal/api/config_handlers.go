package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/vision"
)

// GetConfig handles GET /api/config — returns the current runtime config.
func (h *Handlers) GetConfig(c echo.Context) error {
	return c.JSON(http.StatusOK, h.cfg)
}

// GetVisionConfig handles GET /api/config/vision — returns vision config.
func (h *Handlers) GetVisionConfig(c echo.Context) error {
	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()
	return c.JSON(http.StatusOK, h.cfg.Vision)
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

	h.log.Info(
		"vision config updated",
		"url", req.URL,
		"model", req.Model,
		"has_prompt", req.Prompt != "",
	)
	return c.JSON(http.StatusOK, req)
}

// ListTTSPresets handles GET /api/config/tts — returns all TTS presets.
func (h *Handlers) ListTTSPresets(c echo.Context) error {
	presets, err := config.ListTTSPresets(h.db)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"presets": presets,
		"active":  h.cfg.TTS,
	})
}

// CreateTTSPreset handles POST /api/config/tts — creates a new TTS preset.
func (h *Handlers) CreateTTSPreset(c echo.Context) error {
	var p config.TTSPreset
	if err := c.Bind(&p); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	if p.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if p.Endpoint == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "endpoint is required")
	}
	if err := config.SaveTTSPreset(h.db, p); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, p)
}

// UpdateTTSPreset handles PUT /api/config/tts/:name — updates an existing TTS preset.
func (h *Handlers) UpdateTTSPreset(c echo.Context) error {
	name := c.Param("name")
	var p config.TTSPreset
	if err := c.Bind(&p); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	p.Name = name
	if err := config.SaveTTSPreset(h.db, p); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, p)
}

// DeleteTTSPreset handles DELETE /api/config/tts/:name — deletes a TTS preset.
func (h *Handlers) DeleteTTSPreset(c echo.Context) error {
	name := c.Param("name")
	if err := config.DeleteTTSPreset(h.db, name); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"deleted": name})
}

// ActivateTTSPreset handles POST /api/config/tts/:name/activate — sets the active TTS preset.
func (h *Handlers) ActivateTTSPreset(c echo.Context) error {
	name := c.Param("name")
	if err := config.SetActiveTTSPreset(h.db, name); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	// Reload the active TTS config into the running config
	presets, err := config.ListTTSPresets(h.db)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.cfgMu.Lock()
	for _, p := range presets {
		if p.IsActive {
			h.cfg.TTS = config.TTSConfig{
				URL:          p.Endpoint,
				Model:        p.Model,
				DefaultVoice: p.DefaultVoice,
				APIKey:       p.APIKey,
			}
			break
		}
	}
	h.cfgMu.Unlock()
	return c.JSON(http.StatusOK, map[string]string{"active": name})
}

// ListCamerasConfig handles GET /api/config/cameras — returns all configured cameras.
func (h *Handlers) ListCamerasConfig(c echo.Context) error {
	apStatus := map[string]bool{}
	if h.airplayMgr != nil {
		apStatus = h.airplayMgr.Status()
	}
	cameras := make([]map[string]interface{}, 0, len(h.cfg.Cameras))
	for name, cam := range h.cfg.Cameras {
		cameras = append(cameras, map[string]interface{}{
			"name":            name,
			"type":            cam.Type,
			"ip":              cam.IP,
			"channel":         cam.Channel,
			"stream":          cam.Stream,
			"enabled":         cam.Enabled,
			"airplay_enabled": cam.AirPlayEnabled,
			"airplay_running": apStatus[name],
			"vision_prompt":   cam.VisionPrompt,
		})
	}
	return c.JSON(http.StatusOK, cameras)
}

// CreateCamera handles POST /api/config/cameras — adds or updates a camera.
func (h *Handlers) CreateCamera(c echo.Context) error {
	var req struct {
		Name         string `json:"name"`
		Type         string `json:"type"`
		IP           string `json:"ip"`
		User         string `json:"user"`
		Pass         string `json:"pass"`
		Channel      int    `json:"channel"`
		Stream       string `json:"stream"`
		Enabled      *bool  `json:"enabled"` // pointer so we can distinguish unset from false
		VisionPrompt string `json:"vision_prompt"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	if req.Name == "" || req.IP == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name and ip are required")
	}
	if req.Type == "" {
		req.Type = "hikvision"
	}
	if req.Channel == 0 {
		req.Channel = 1
	}
	// If editing an existing camera and enabled isn't specified, preserve current value
	enabled := false
	if req.Enabled != nil {
		enabled = *req.Enabled
	} else if existing, ok := h.cfg.Cameras[req.Name]; ok {
		enabled = existing.Enabled
	}
	// Preserve existing vision_prompt if not provided
	visionPrompt := req.VisionPrompt
	if visionPrompt == "" {
		if existing, ok := h.cfg.Cameras[req.Name]; ok {
			visionPrompt = existing.VisionPrompt
		}
	}
	cam := config.CameraConfig{
		Type:         req.Type,
		IP:           req.IP,
		User:         req.User,
		Pass:         req.Pass,
		Channel:      req.Channel,
		Stream:       req.Stream,
		Enabled:      enabled,
		VisionPrompt: visionPrompt,
	}
	if err := config.SaveCamera(h.db, req.Name, cam); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	// Update running config + registry
	h.cfg.Cameras[req.Name] = cam
	h.reg.UpdateConfig(req.Name, cam)
	if cam.Enabled {
		if err := h.reg.EnableCamera(req.Name, cam); err != nil {
			h.log.Error("camera enable failed", "name", req.Name, "err", err)
		}
	} else {
		h.reg.DisableCamera(req.Name)
	}
	h.log.Info("camera saved", "name", req.Name, "type", req.Type, "enabled", enabled)
	return c.JSON(http.StatusCreated, map[string]interface{}{
		"name":          req.Name,
		"type":          req.Type,
		"ip":            req.IP,
		"channel":       req.Channel,
		"stream":        req.Stream,
		"enabled":       enabled,
		"vision_prompt": visionPrompt,
	})
}

// DeleteCameraConfig handles DELETE /api/config/cameras/:name — removes a camera.
func (h *Handlers) DeleteCameraConfig(c echo.Context) error {
	name := c.Param("name")
	if err := config.DeleteCamera(h.db, name); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	delete(h.cfg.Cameras, name)
	h.reg.DisableCamera(name)
	return c.JSON(http.StatusOK, map[string]string{"deleted": name})
}

// ToggleCamera handles PATCH /api/config/cameras/:name/toggle — enables/disables a camera.
func (h *Handlers) ToggleCamera(c echo.Context) error {
	name := c.Param("name")
	cam, ok := h.cfg.Cameras[name]
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "camera not found")
	}
	cam.Enabled = !cam.Enabled
	if err := config.SaveCamera(h.db, name, cam); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.cfg.Cameras[name] = cam
	h.reg.UpdateConfig(name, cam)
	if cam.Enabled {
		if err := h.reg.EnableCamera(name, cam); err != nil {
			h.log.Error("camera enable failed", "name", name, "err", err)
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if h.airplayMgr != nil && cam.AirPlayEnabled {
			if err := h.airplayMgr.Enable(name); err != nil {
				h.log.Warn("AirPlay enable failed", "camera", name, "err", err)
			}
		}
	} else {
		h.reg.DisableCamera(name)
		if h.airplayMgr != nil {
			h.airplayMgr.Disable(name)
		}
	}
	h.log.Info("camera toggled", "name", name, "enabled", cam.Enabled)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"name":    name,
		"enabled": cam.Enabled,
	})
}

// ListVisionPrompts handles GET /api/config/vision-prompts — returns all saved vision prompts.
func (h *Handlers) ListVisionPrompts(c echo.Context) error {
	prompts, err := config.ListVisionPrompts(h.db)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, prompts)
}

// CreateVisionPrompt handles POST /api/config/vision-prompts — creates or updates a vision prompt.
func (h *Handlers) CreateVisionPrompt(c echo.Context) error {
	var p config.VisionPrompt
	if err := c.Bind(&p); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	if p.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if err := config.SaveVisionPrompt(h.db, p); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.log.Info("vision prompt saved", "name", p.Name)
	return c.JSON(http.StatusCreated, p)
}

// DeleteVisionPrompt handles DELETE /api/config/vision-prompts/:name — removes a vision prompt.
func (h *Handlers) DeleteVisionPrompt(c echo.Context) error {
	name := c.Param("name")
	if err := config.DeleteVisionPrompt(h.db, name); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.log.Info("vision prompt deleted", "name", name)
	return c.JSON(http.StatusOK, map[string]string{"deleted": name})
}

// ListRules handles GET /api/config/rules — returns all MQTT rules.
func (h *Handlers) ListRules(c echo.Context) error {
	return c.JSON(http.StatusOK, h.cfg.Rules)
}

// CreateRule handles POST /api/config/rules — creates a new MQTT rule.
func (h *Handlers) CreateRule(c echo.Context) error {
	var r config.Rule
	if err := c.Bind(&r); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	if r.Topic == "" {
		r.Topic = "frigate/events"
	}
	// Serialize filter and cameras for SQLite
	filterJSON, err := json.Marshal(r.Filter)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	camerasCSV := strings.Join(r.Cameras, ",")
	enabled := 1
	if !r.Enabled {
		enabled = 0
	}
	result, err := h.db.Exec(
		`INSERT INTO rules (topic, filter, cameras, preset, text, voice, enabled)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.Topic, string(filterJSON), camerasCSV, r.Preset, r.Text, r.Voice, enabled,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	id, err := result.LastInsertId()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	r.ID = int(id)
	return c.JSON(http.StatusCreated, r)
}

// GetAirPlayConfig handles GET /api/config/airplay — returns AirPlay config and per-camera status.
func (h *Handlers) GetAirPlayConfig(c echo.Context) error {
	h.cfgMu.Lock()
	ap := h.cfg.AirPlay
	cams := h.cfg.Cameras
	h.cfgMu.Unlock()

	status := map[string]bool{}
	if h.airplayMgr != nil {
		status = h.airplayMgr.Status()
	}

	perCamera := make([]map[string]interface{}, 0, len(cams))
	for name, cam := range cams {
		perCamera = append(perCamera, map[string]interface{}{
			"name":            name,
			"airplay_enabled": cam.AirPlayEnabled,
			"airplay_running": status[name],
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"enabled":    ap.Enabled,
		"base_port":  ap.BasePort,
		"per_camera": perCamera,
	})
}

// ToggleAirPlay handles PATCH /api/config/airplay/:camera/toggle —
// enables or disables the shairport-sync receiver for a single camera live.
func (h *Handlers) ToggleAirPlay(c echo.Context) error {
	name := c.Param("camera")

	h.cfgMu.Lock()
	cam, ok := h.cfg.Cameras[name]
	if !ok {
		h.cfgMu.Unlock()
		return echo.NewHTTPError(http.StatusNotFound, "camera not found")
	}
	cam.AirPlayEnabled = !cam.AirPlayEnabled
	h.cfg.Cameras[name] = cam
	h.cfgMu.Unlock()

	if err := config.SaveCamera(h.db, name, cam); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	running := false
	if h.airplayMgr != nil {
		if cam.AirPlayEnabled && cam.Enabled {
			if err := h.airplayMgr.Enable(name); err != nil {
				h.log.Warn("AirPlay enable failed", "camera", name, "err", err)
			}
		} else {
			h.airplayMgr.Disable(name)
		}
		running = h.airplayMgr.IsRunning(name)
	}

	h.log.Info(
		"AirPlay toggled",
		"camera",
		name,
		"airplay_enabled",
		cam.AirPlayEnabled,
		"running",
		running,
	)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"camera":          name,
		"airplay_enabled": cam.AirPlayEnabled,
		"running":         running,
	})
}

// UpdateAirPlayConfig handles PUT /api/config/airplay — updates AirPlay config.
// Note: changing these settings requires a server restart to take effect
// (AirPlay receivers are started at boot time).
func (h *Handlers) UpdateAirPlayConfig(c echo.Context) error {
	var req config.AirPlayConfig
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}

	enabled := "0"
	if req.Enabled {
		enabled = "1"
	}
	if req.BasePort == 0 {
		req.BasePort = 5000
	}

	if err := config.SetPreference(h.db, "airplay_enabled", enabled); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := config.SetPreference(h.db, "airplay_base_port",
		fmt.Sprintf("%d", req.BasePort)); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	h.cfgMu.Lock()
	h.cfg.AirPlay = req
	h.cfgMu.Unlock()

	h.log.Info("AirPlay config updated", "enabled", req.Enabled, "basePort", req.BasePort)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"enabled":   req.Enabled,
		"base_port": req.BasePort,
		"note":      "restart required for changes to take effect",
	})
}

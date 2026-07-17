package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/jeeftor/camspeak/internal/config"
)

// GetConfig handles GET /api/config — returns the current runtime config.
func (h *Handlers) GetConfig(c echo.Context) error {
	return c.JSON(http.StatusOK, h.cfg)
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
	cameras := make([]map[string]interface{}, 0, len(h.cfg.Cameras))
	for name, cam := range h.cfg.Cameras {
		cameras = append(cameras, map[string]interface{}{
			"name":    name,
			"type":    cam.Type,
			"ip":      cam.IP,
			"channel": cam.Channel,
			"stream":  cam.Stream,
			"enabled": cam.Enabled,
		})
	}
	return c.JSON(http.StatusOK, cameras)
}

// CreateCamera handles POST /api/config/cameras — adds or updates a camera.
func (h *Handlers) CreateCamera(c echo.Context) error {
	var req struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		IP      string `json:"ip"`
		User    string `json:"user"`
		Pass    string `json:"pass"`
		Channel int    `json:"channel"`
		Stream  string `json:"stream"`
		Enabled *bool  `json:"enabled"` // pointer so we can distinguish unset from false
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
	cam := config.CameraConfig{
		Type:    req.Type,
		IP:      req.IP,
		User:    req.User,
		Pass:    req.Pass,
		Channel: req.Channel,
		Stream:  req.Stream,
		Enabled: enabled,
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
		"name":    req.Name,
		"type":    req.Type,
		"ip":      req.IP,
		"channel": req.Channel,
		"stream":  req.Stream,
		"enabled": enabled,
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
	} else {
		h.reg.DisableCamera(name)
	}
	h.log.Info("camera toggled", "name", name, "enabled", cam.Enabled)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"name":    name,
		"enabled": cam.Enabled,
	})
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

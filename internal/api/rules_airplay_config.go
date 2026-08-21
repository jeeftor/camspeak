package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/jeeftor/camspeak/internal/config"
)

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
		`INSERT INTO rules (topic, filter, cameras, preset, text, voice, loop, enabled)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Topic, string(filterJSON), camerasCSV, r.Preset, r.Text, r.Voice, r.Loop, enabled,
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
			"airplay_name":    cam.AirPlayName,
			"airplay_model":   cam.AirPlayModel,
			"airplay_running": status[name],
		})
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"enabled":          ap.Enabled,
		"base_port":        ap.BasePort,
		"prime_silence_ms": ap.PrimeSilenceMs,
		"model":            ap.Model,
		"gain":             ap.Gain,
		"per_camera":       perCamera,
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
				h.logger(c).Warn("AirPlay enable failed", "camera", name, "err", err)
			}
		} else {
			h.airplayMgr.Disable(name)
		}
		running = h.airplayMgr.IsRunning(name)
	}

	h.logger(c).Info(
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
	if req.Model == "" {
		req.Model = "RealityDevice14,1"
	}
	if req.Gain == 0 {
		req.Gain = 1.0
	}

	if err := config.SetPreference(h.db, "airplay_enabled", enabled); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := config.SetPreference(h.db, "airplay_base_port",
		fmt.Sprintf("%d", req.BasePort)); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if req.PrimeSilenceMs < 0 {
		req.PrimeSilenceMs = 0
	}
	if err := config.SetPreference(h.db, "airplay_prime_silence_ms",
		fmt.Sprintf("%d", req.PrimeSilenceMs)); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := config.SetPreference(h.db, "airplay_model", req.Model); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := config.SetPreference(h.db, "airplay_gain",
		fmt.Sprintf("%f", req.Gain)); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	h.cfgMu.Lock()
	oldModel := h.cfg.AirPlay.Model
	oldGain := h.cfg.AirPlay.Gain
	h.cfg.AirPlay = req
	h.cfgMu.Unlock()

	// Restart running AirPlay receivers if the model or gain changed so the
	// new mDNS records are advertised and the new gain takes effect.
	if h.airplayMgr != nil && (oldModel != req.Model || oldGain != req.Gain) {
		h.airplayMgr.RestartRunning()
	}

	h.logger(c).Info(
		"AirPlay config updated",
		"enabled",
		req.Enabled,
		"basePort",
		req.BasePort,
		"primeSilenceMs",
		req.PrimeSilenceMs,
		"model",
		req.Model,
		"gain",
		req.Gain,
	)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"enabled":          req.Enabled,
		"base_port":        req.BasePort,
		"prime_silence_ms": req.PrimeSilenceMs,
		"model":            req.Model,
		"gain":             req.Gain,
		"note":             "running receivers restarted to apply model/gain changes; port/enabled changes require server restart",
	})
}

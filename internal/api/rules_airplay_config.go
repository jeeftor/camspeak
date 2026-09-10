package api

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jeeftor/camspeak/internal/config"
)

// GetAirPlayConfig handles GET /api/config/airplay — returns AirPlay config and per-camera status.
func (h *Handlers) GetAirPlayConfig(c echo.Context) error {
	cfg := h.configSnapshot()
	ap, cams := cfg.AirPlay, cfg.Cameras

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
	h.configEditMu.Lock()
	defer h.configEditMu.Unlock()
	name := c.Param("camera")

	cam, ok := h.configSnapshot().Cameras[name]
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "camera not found")
	}
	cam.AirPlayEnabled = !cam.AirPlayEnabled

	if err := config.SaveCamera(h.db, name, cam); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.cfgMu.Lock()
	h.cfg.Cameras[name] = cam
	h.cfgMu.Unlock()
	h.reg.UpdateConfig(name, cam)

	running := false
	if h.airplayMgr != nil {
		if err := h.airplayMgr.UpdateCamera(name, cam); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
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
// Existing receivers are restarted when their configuration changes.
func (h *Handlers) UpdateAirPlayConfig(c echo.Context) error {
	h.configEditMu.Lock()
	defer h.configEditMu.Unlock()
	var req config.AirPlayConfig
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}

	enabled := "0"
	if req.Enabled {
		enabled = "1"
	}
	if req.BasePort == 0 {
		req.BasePort = 5100
	}
	if req.Model == "" {
		req.Model = "RealityDevice14,1"
	}
	if req.Gain < 0 || req.Gain > 10 || req.BasePort < 1024 || req.BasePort > 65535 {
		return echo.NewHTTPError(
			http.StatusBadRequest,
			"gain must be between 0 and 10 and base_port between 1024 and 65535",
		)
	}

	if req.PrimeSilenceMs < 0 {
		req.PrimeSilenceMs = 0
	}
	if err := config.SetPreferences(h.db, map[string]string{
		"airplay_enabled":          enabled,
		"airplay_base_port":        fmt.Sprintf("%d", req.BasePort),
		"airplay_prime_silence_ms": fmt.Sprintf("%d", req.PrimeSilenceMs),
		"airplay_model":            req.Model,
		"airplay_gain":             fmt.Sprintf("%f", req.Gain),
	}); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	h.cfgMu.Lock()
	h.cfg.AirPlay = req
	config.ApplyEnvOverrides(h.cfg)
	req = h.cfg.AirPlay
	h.cfgMu.Unlock()

	// Restart running AirPlay receivers if the model or gain changed so the
	// new mDNS records are advertised and the new gain takes effect.
	if h.airplayMgr != nil {
		if err := h.airplayMgr.UpdateConfig(req); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
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
		"note":             "AirPlay receivers updated",
	})
}

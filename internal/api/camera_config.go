package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/frigate"
)

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
			"user":            cam.User,
			"channel":         cam.Channel,
			"stream":          cam.Stream,
			"enabled":         cam.Enabled,
			"airplay_enabled": cam.AirPlayEnabled,
			"airplay_name":    cam.AirPlayName,
			"airplay_model":   cam.AirPlayModel,
			"gain":            cam.Gain,
			"airplay_running": apStatus[name],
			"vision_prompt":   cam.VisionPrompt,
			"note":            cam.Note,
		})
	}
	return c.JSON(http.StatusOK, cameras)
}

// CreateCamera handles POST /api/config/cameras — adds or updates a camera.
func (h *Handlers) CreateCamera(c echo.Context) error {
	log := h.logger(c)
	var req struct {
		Name         string  `json:"name"`
		Type         string  `json:"type"`
		IP           string  `json:"ip"`
		User         string  `json:"user"`
		Pass         string  `json:"pass"`
		Channel      int     `json:"channel"`
		Stream       string  `json:"stream"`
		Enabled      *bool   `json:"enabled"` // pointer so we can distinguish unset from false
		AirPlayName  string  `json:"airplay_name"`
		AirPlayModel string  `json:"airplay_model"`
		Gain         float64 `json:"gain"`
		VisionPrompt string  `json:"vision_prompt"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	if req.Name == "" || req.IP == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name and ip are required")
	}
	existing, hasExisting := h.cfg.Cameras[req.Name]

	// If editing an existing camera and enabled isn't specified, preserve current value.
	enabled := false
	if req.Enabled != nil {
		enabled = *req.Enabled
	} else if hasExisting {
		enabled = existing.Enabled
	}

	// Preserve existing camera fields when the request doesn't provide them.
	// This matters because the UI does not return secrets (passwords) in API responses.
	camType := req.Type
	if camType == "" && hasExisting {
		camType = existing.Type
	}
	user := req.User
	if user == "" && hasExisting {
		user = existing.User
	}
	pass := req.Pass
	if pass == "" && hasExisting {
		pass = existing.Pass
	}
	channel := req.Channel
	if channel == 0 && hasExisting {
		channel = existing.Channel
	}
	if channel == 0 {
		channel = 1
	}
	stream := req.Stream
	if stream == "" && hasExisting {
		stream = existing.Stream
	}

	// Auto-detect camera type only when adding a new camera and type is not provided.
	if camType == "" {
		detected := cameras.ProbeCameraType(req.IP, user, pass)
		if detected != "" {
			camType = detected
			log.Info("camera type auto-detected", "camera", req.Name, "type", detected)
		} else {
			camType = "hikvision"
		}
	}
	// For Reolink cameras, default stream name to the camera name so audio can be
	// routed through go2rtc if a go2rtc instance is reachable.
	if camType == "reolink" && stream == "" {
		stream = req.Name
	}

	// Preserve per-camera AirPlay fields if not provided in this request.
	airPlayName := req.AirPlayName
	if airPlayName == "" && hasExisting {
		airPlayName = existing.AirPlayName
	}
	airPlayModel := req.AirPlayModel
	if airPlayModel == "" && hasExisting {
		airPlayModel = existing.AirPlayModel
	}
	gain := req.Gain
	if gain == 0 && hasExisting {
		gain = existing.Gain
	}
	if gain == 0 {
		gain = 3.0
	}

	// Preserve existing vision_prompt if not provided
	visionPrompt := req.VisionPrompt
	if visionPrompt == "" && hasExisting {
		visionPrompt = existing.VisionPrompt
	}
	// Auto-set limitation note for Reolink cameras (native audio not implemented).
	note := ""
	if camType == "reolink" {
		note = "Limited — Reolink audio requires go2rtc with " +
			"#backchannel=1 (doorbells only, firmware-dependent)"
	}
	// Preserve existing note if not a Reolink camera and no new note applies.
	if note == "" && hasExisting {
		note = existing.Note
	}
	cam := config.CameraConfig{
		Type:         camType,
		IP:           req.IP,
		User:         user,
		Pass:         pass,
		Channel:      channel,
		Stream:       stream,
		Enabled:      enabled,
		AirPlayName:  airPlayName,
		AirPlayModel: airPlayModel,
		Gain:         gain,
		VisionPrompt: visionPrompt,
		Note:         note,
	}
	if err := config.SaveCamera(h.db, req.Name, cam); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	// Update running config + registry
	h.cfg.Cameras[req.Name] = cam
	h.reg.UpdateConfig(req.Name, cam)
	if cam.Enabled {
		if err := h.reg.EnableCamera(req.Name, cam); err != nil {
			h.logger(c).Error("camera enable failed", "name", req.Name, "err", err)
		}
	} else {
		h.reg.DisableCamera(req.Name)
		if h.airplayMgr != nil {
			h.airplayMgr.Disable(req.Name)
		}
	}
	// If AirPlay name/model/gain changed, restart the receiver so the new mDNS records are advertised
	// and the new gain takes effect on the next stream.
	airplayChanged := hasExisting &&
		(existing.AirPlayName != cam.AirPlayName || existing.AirPlayModel != cam.AirPlayModel || existing.Gain != cam.Gain)
	if h.airplayMgr != nil && cam.AirPlayEnabled && cam.Enabled && airplayChanged {
		h.airplayMgr.Disable(req.Name)
		if err := h.airplayMgr.Enable(req.Name); err != nil {
			h.logger(c).
				Warn("AirPlay restart after name/model/gain change failed", "camera", req.Name, "err", err)
		}
	}
	h.logger(c).Info(
		"camera saved",
		"name",
		req.Name,
		"type",
		req.Type,
		"enabled",
		enabled,
		"airplay_name",
		cam.AirPlayName,
		"airplay_model",
		cam.AirPlayModel,
	)
	return c.JSON(http.StatusCreated, map[string]interface{}{
		"name":          req.Name,
		"type":          req.Type,
		"ip":            req.IP,
		"channel":       req.Channel,
		"stream":        req.Stream,
		"enabled":       enabled,
		"airplay_name":  cam.AirPlayName,
		"airplay_model": cam.AirPlayModel,
		"vision_prompt": visionPrompt,
	})
}

// DetectCameraType handles POST /api/config/cameras/detect — probes a camera
// and returns the detected vendor type plus any reachable go2rtc URL.
func (h *Handlers) DetectCameraType(c echo.Context) error {
	log := h.logger(c)
	var req struct {
		IP   string `json:"ip"`
		User string `json:"user"`
		Pass string `json:"pass"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	if req.IP == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "ip is required")
	}

	detected := cameras.ProbeCameraType(req.IP, req.User, req.Pass)
	go2rtcURL := h.cfg.Go2rtcURL
	if go2rtcURL == "" {
		go2rtcURL = cameras.FindGo2rtcURL(h.cfg.FrigateURL)
	}
	if go2rtcURL != "" {
		log.Debug("detected go2rtc for camera probe", "url", go2rtcURL)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"ip":         req.IP,
		"type":       detected,
		"go2rtc_url": go2rtcURL,
		"note": func() string {
			if detected == "reolink" {
				if go2rtcURL != "" {
					return "Reolink — LIMITED. Needs go2rtc stream with " +
						"#backchannel=1 (doorbells only, firmware-dependent)"
				}
				return "Reolink — LIMITED. Native protocol not implemented. " +
					"Needs go2rtc with #backchannel=1 (doorbells only)"
			}
			return ""
		}(),
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
			h.logger(c).Error("camera enable failed", "name", name, "err", err)
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if h.airplayMgr != nil && cam.AirPlayEnabled {
			if err := h.airplayMgr.Enable(name); err != nil {
				h.logger(c).Warn("AirPlay enable failed", "camera", name, "err", err)
			}
		}
	} else {
		h.reg.DisableCamera(name)
		if h.airplayMgr != nil {
			h.airplayMgr.Disable(name)
		}
	}
	h.logger(c).Info("camera toggled", "name", name, "enabled", cam.Enabled)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"name":    name,
		"enabled": cam.Enabled,
	})
}

// ListGo2rtcStreamsHandler handles GET /api/config/go2rtc/streams — lists
// all streams configured in go2rtc so the UI can show available stream names
// when configuring a Reolink camera.
func (h *Handlers) ListGo2rtcStreams(c echo.Context) error {
	h.cfgMu.Lock()
	go2rtcURL := h.cfg.Go2rtcURL
	h.cfgMu.Unlock()

	if go2rtcURL == "" {
		go2rtcURL = cameras.FindGo2rtcURL(h.cfg.FrigateURL)
	}
	if go2rtcURL == "" {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"go2rtc_url": "",
			"streams":    []interface{}{},
			"note":       "go2rtc URL not configured — set it in Config → Settings",
		})
	}

	streams, err := cameras.ListGo2rtcStreams(go2rtcURL)
	if err != nil {
		h.logger(c).Warn("failed to list go2rtc streams", "url", go2rtcURL, "err", err)
		return c.JSON(http.StatusOK, map[string]interface{}{
			"go2rtc_url": go2rtcURL,
			"streams":    []interface{}{},
			"error":      err.Error(),
		})
	}

	type streamInfo struct {
		Name           string `json:"name"`
		Source         string `json:"source"`
		HasBackchannel bool   `json:"has_backchannel"`
	}
	result := make([]streamInfo, 0, len(streams))
	for name, src := range streams {
		result = append(result, streamInfo{
			Name:           name,
			Source:         src,
			HasBackchannel: strings.Contains(src, "backchannel"),
		})
	}
	h.logger(c).Debug("listed go2rtc streams", "url", go2rtcURL, "count", len(result))
	return c.JSON(http.StatusOK, map[string]interface{}{
		"go2rtc_url": go2rtcURL,
		"streams":    result,
	})
}

// CameraInfoHandler handles GET /api/cameras/:name/info — queries the camera's
// vendor API (ISAPI for Hikvision, SOAP for ONVIF) and returns device info,
// streaming configuration (codec, resolution, framerate, bitrate), and network info.
func (h *Handlers) CameraInfoHandler(c echo.Context) error {
	log := h.logger(c)
	name := c.Param("name")
	h.cfgMu.Lock()
	cam, ok := h.cfg.Cameras[name]
	h.cfgMu.Unlock()
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "camera not found")
	}

	info, err := cameras.QueryCameraInfo(cam)
	if err != nil {
		log.Warn("camera info query failed", "camera", name, "err", err)
		// Return partial info with errors rather than a hard 500 — the UI can
		// still show whatever fields were successfully retrieved.
		if !info.Online && len(info.Streams) == 0 && info.Device.Manufacturer == "" {
			return echo.NewHTTPError(http.StatusBadGateway, err.Error())
		}
	}
	log.Debug("camera info queried", "camera", name, "type", cam.Type, "streams", len(info.Streams))
	return c.JSON(http.StatusOK, info)
}

// DiscoverCameras handles POST /api/cameras/discover — queries Frigate for cameras,
// saves them to the database, and returns the discovered list.
func (h *Handlers) DiscoverCameras(c echo.Context) error {
	h.cfgMu.Lock()
	frigateURL := h.cfg.FrigateURL
	h.cfgMu.Unlock()

	if frigateURL == "" {
		return echo.NewHTTPError(http.StatusServiceUnavailable,
			"frigate_url not configured — set it in Config → Settings")
	}

	d := frigate.NewDiscoverer(frigateURL)
	cameras, err := d.Discover()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway,
			fmt.Sprintf("frigate discovery failed: %s", err))
	}
	if len(cameras) == 0 {
		return c.JSON(http.StatusOK, map[string]interface{}{
			"discovered": 0,
			"cameras":    []interface{}{},
			"note":       "no cameras found in Frigate config",
		})
	}

	if err := frigate.SaveToDB(h.db, cameras); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Reload config so the new cameras are live
	h.cfgMu.Lock()
	for _, cam := range cameras {
		stream := cam.Stream
		note := ""
		// For Reolink cameras, default the go2rtc stream name to the camera name.
		if cam.Type == "reolink" {
			note = "Limited — Reolink audio requires go2rtc with " +
				"#backchannel=1 (doorbells only, firmware-dependent)"
			if stream == "" {
				stream = cam.Name
			}
		}
		h.cfg.Cameras[cam.Name] = config.CameraConfig{
			Type:    cam.Type,
			IP:      cam.IP,
			User:    cam.User,
			Pass:    cam.Pass,
			Channel: cam.Channel,
			Stream:  stream,
			Enabled: true,
			Note:    note,
		}
	}
	h.cfgMu.Unlock()

	// Persist the note for cameras that have one (SaveToDB doesn't include the note column).
	for name, cam := range h.cfg.Cameras {
		if cam.Note != "" {
			_ = config.SaveCamera(h.db, name, cam)
		}
	}

	h.logger(c).Info("cameras discovered via Frigate", "count", len(cameras), "frigate", frigateURL)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"discovered": len(cameras),
		"cameras":    cameras,
	})
}

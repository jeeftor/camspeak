package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/frigate"
	"github.com/jeeftor/camspeak/internal/util"
)

// ListCamerasConfig handles GET /api/config/cameras — returns all configured cameras.
func (h *Handlers) ListCamerasConfig(c echo.Context) error {
	apStatus := map[string]bool{}
	if h.airplayMgr != nil {
		apStatus = h.airplayMgr.Status()
	}
	cfg := h.configSnapshot()
	cameras := make([]map[string]interface{}, 0, len(cfg.Cameras))
	for name, cam := range cfg.Cameras {
		sc := cam.Sanitized()
		cameras = append(cameras, map[string]interface{}{
			"name":            name,
			"type":            sc.Type,
			"ip":              sc.IP,
			"user":            sc.User,
			"channel":         sc.Channel,
			"stream":          sc.Stream,
			"enabled":         sc.Enabled,
			"airplay_enabled": sc.AirPlayEnabled,
			"airplay_name":    sc.AirPlayName,
			"airplay_model":   sc.AirPlayModel,
			"gain":            sc.Gain,
			"airplay_running": apStatus[name],
			"vision_prompt":   sc.VisionPrompt,
			"vision_stream":   sc.VisionStream,
			"vision_width":    sc.VisionWidth,
			"snap_method":     sc.SnapMethod,
			"tts_mode":        sc.TTSMode,
			"note":            sc.Note,
			"sort_order":      sc.SortOrder,
		})
	}
	return c.JSON(http.StatusOK, cameras)
}

// sortedCameraNames returns camera names sorted by sort_order (ascending),
// then alphabetically for cameras with the same (or zero) sort_order.
func (h *Handlers) sortedCameraNames() []string {
	type camSort struct {
		name  string
		order int
	}
	cfg := h.configSnapshot()
	cams := make([]camSort, 0, len(cfg.Cameras))
	for name, cfg := range cfg.Cameras {
		cams = append(cams, camSort{name: name, order: cfg.SortOrder})
	}
	// Sort by order, then by name for ties (or when order is 0).
	sort.Slice(cams, func(i, j int) bool {
		return cams[i].order < cams[j].order ||
			(cams[i].order == cams[j].order && cams[i].name < cams[j].name)
	})
	names := make([]string, len(cams))
	for i, c := range cams {
		names[i] = c.name
	}
	return names
}

// ReorderCameras handles POST /api/config/cameras/reorder — sets sort_order
// for each camera based on the provided name list.
func (h *Handlers) ReorderCameras(c echo.Context) error {
	h.configEditMu.Lock()
	defer h.configEditMu.Unlock()
	var req struct {
		Cameras []string `json:"cameras"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	if len(req.Cameras) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "cameras list required")
	}

	h.cfgMu.Lock()
	defer h.cfgMu.Unlock()

	for i, name := range req.Cameras {
		cam, ok := h.cfg.Cameras[name]
		if !ok {
			continue
		}
		cam.SortOrder = i + 1 // 1-based so 0 remains "unset"
		if err := config.SaveCamera(h.db, name, cam); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		h.cfg.Cameras[name] = cam
		h.reg.UpdateConfig(name, cam)
	}

	h.logger(c).Info("cameras reordered", "order", req.Cameras)
	return c.JSON(http.StatusOK, map[string]any{"status": "ok"})
}

// CreateCamera handles POST /api/config/cameras — adds or updates a camera.
func (h *Handlers) CreateCamera(c echo.Context) error {
	h.configEditMu.Lock()
	defer h.configEditMu.Unlock()
	log := h.logger(c)
	var req struct {
		Name           string   `json:"name"`
		Type           string   `json:"type"`
		IP             string   `json:"ip"`
		User           *string  `json:"user"`
		Pass           string   `json:"pass"`
		Channel        int      `json:"channel"`
		Stream         *string  `json:"stream"`
		Enabled        *bool    `json:"enabled"` // pointer so we can distinguish unset from false
		AirPlayEnabled *bool    `json:"airplay_enabled"`
		AirPlayName    *string  `json:"airplay_name"`
		AirPlayModel   *string  `json:"airplay_model"`
		Gain           *float64 `json:"gain"`
		VisionPrompt   *string  `json:"vision_prompt"`
		VisionStream   *string  `json:"vision_stream"`
		VisionWidth    *int     `json:"vision_width"`
		SnapMethod     *string  `json:"snap_method"`
		TTSMode        *string  `json:"tts_mode"`
		ClearPassword  bool     `json:"clear_password"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	if req.Name == "" || req.IP == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name and ip are required")
	}
	existing, hasExisting := h.configSnapshot().Cameras[req.Name]

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
	user := stringUpdate(req.User, existing.User)
	pass := req.Pass
	if pass == "" && hasExisting && !req.ClearPassword {
		pass = existing.Pass
	}
	channel := req.Channel
	if channel == 0 && hasExisting {
		channel = existing.Channel
	}
	if channel == 0 {
		channel = 1
	}
	stream := stringUpdate(req.Stream, existing.Stream)

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
	if camType == "reolink" && stream == "" && !hasExisting && req.Stream == nil {
		stream = req.Name
	}

	// Preserve per-camera AirPlay fields if not provided in this request.
	airPlayEnabled := true
	if req.AirPlayEnabled != nil {
		airPlayEnabled = *req.AirPlayEnabled
	} else if hasExisting {
		airPlayEnabled = existing.AirPlayEnabled
	}
	airPlayName := stringUpdate(req.AirPlayName, existing.AirPlayName)
	airPlayModel := stringUpdate(req.AirPlayModel, existing.AirPlayModel)
	gain := 3.0
	if hasExisting {
		gain = existing.Gain
	}
	if req.Gain != nil {
		gain = *req.Gain
	}
	if gain < 0 || gain > 10 {
		return echo.NewHTTPError(http.StatusBadRequest, "gain must be between 0 and 10")
	}

	// Preserve existing vision_prompt if not provided
	visionPrompt := stringUpdate(req.VisionPrompt, existing.VisionPrompt)
	// Preserve existing vision_stream if not provided
	visionStream := stringUpdate(req.VisionStream, existing.VisionStream)
	// Preserve existing vision_width if not provided
	visionWidth := existing.VisionWidth
	if req.VisionWidth != nil {
		visionWidth = *req.VisionWidth
	}
	// Preserve existing snap_method if not provided
	snapMethod := stringUpdate(req.SnapMethod, existing.SnapMethod)
	ttsMode := stringUpdate(req.TTSMode, existing.TTSMode)
	if ttsMode != "" && ttsMode != "buffered" && ttsMode != "streaming" {
		return echo.NewHTTPError(http.StatusBadRequest, "tts_mode must be empty, buffered or streaming")
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
		Type:           camType,
		IP:             req.IP,
		User:           user,
		Pass:           pass,
		Channel:        channel,
		Stream:         stream,
		Enabled:        enabled,
		AirPlayEnabled: airPlayEnabled,
		AirPlayName:    airPlayName,
		AirPlayModel:   airPlayModel,
		Gain:           gain,
		VisionPrompt:   visionPrompt,
		VisionStream:   visionStream,
		VisionWidth:    visionWidth,
		SnapMethod:     snapMethod,
		TTSMode:        ttsMode,
		Note:           note,
		SortOrder:      existing.SortOrder,
	}
	if err := h.reg.ValidateConfig(req.Name, cam); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := config.SaveCamera(h.db, req.Name, cam); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := h.applyCamera(req.Name, cam); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
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
		"type":          cam.Type,
		"ip":            cam.IP,
		"channel":       cam.Channel,
		"stream":        cam.Sanitized().Stream,
		"enabled":       enabled,
		"airplay_name":  cam.AirPlayName,
		"airplay_model": cam.AirPlayModel,
		"vision_prompt": visionPrompt,
	})
}

func stringUpdate(value *string, previous string) string {
	if value != nil {
		return *value
	}
	return previous
}

// applyCamera synchronizes the runtime speaker and AirPlay receiver after persistence.
// The caller serializes mutations with configEditMu.
func (h *Handlers) applyCamera(name string, cam config.CameraConfig) error {
	stopOperation(name)
	stopStream(name)
	effective := config.Config{Cameras: map[string]config.CameraConfig{name: cam}}
	config.ApplyEnvOverrides(&effective)
	cam = effective.Cameras[name]
	if h.airplayMgr != nil {
		h.airplayMgr.Disable(name)
	}
	if cam.Enabled {
		if err := h.reg.EnableCamera(name, cam); err != nil {
			return err
		}
	} else {
		h.reg.DisableCamera(name)
	}
	h.reg.UpdateConfig(name, cam)
	h.cfgMu.Lock()
	h.cfg.Cameras[name] = cam
	h.cfgMu.Unlock()
	if h.airplayMgr != nil {
		return h.airplayMgr.UpdateCamera(name, cam)
	}
	return nil
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
	cfg := h.configSnapshot()
	go2rtcURL := cfg.Go2rtcURL
	if go2rtcURL == "" {
		go2rtcURL = cameras.FindGo2rtcURL(cfg.FrigateURL)
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
	h.configEditMu.Lock()
	defer h.configEditMu.Unlock()
	name := c.Param("name")
	if err := config.DeleteCamera(h.db, name); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	stopOperation(name)
	stopStream(name)
	clearPlayback(name)
	if h.airplayMgr != nil {
		h.airplayMgr.RemoveCamera(name)
	}
	h.reg.RemoveCamera(name)
	h.cfgMu.Lock()
	delete(h.cfg.Cameras, name)
	h.cfgMu.Unlock()
	return c.JSON(http.StatusOK, map[string]string{"deleted": name})
}

// SetCameraVolume handles PUT /api/cameras/:name/volume — sets the runtime
// gain for a camera. Takes effect immediately on the next audio chunk
// without restarting playback. Also persists the gain to the camera config
// so it survives restarts.
func (h *Handlers) SetCameraVolume(c echo.Context) error {
	name := c.Param("name")
	log := h.logger(c)

	var req struct {
		Gain float64 `json:"gain"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	if err := h.setCameraGain(name, req.Gain); err != nil {
		return err
	}

	log.Info("volume: set", "camera", name, "gain", req.Gain)
	return c.JSON(http.StatusOK, map[string]any{"camera": name, "gain": req.Gain})
}

// setCameraGain serializes both REST and MCP volume updates with camera edits.
func (h *Handlers) setCameraGain(name string, gain float64) error {
	h.configEditMu.Lock()
	defer h.configEditMu.Unlock()
	if gain < 0 || gain > 10 {
		return echo.NewHTTPError(http.StatusBadRequest, "gain must be between 0 and 10")
	}
	cam, ok := h.configSnapshot().Cameras[name]
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "camera not found")
	}
	cam.Gain = gain
	if err := config.SaveCamera(h.db, name, cam); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.reg.UpdateConfig(name, cam)
	h.cfgMu.Lock()
	h.cfg.Cameras[name] = cam
	h.cfgMu.Unlock()

	return nil
}

// ToggleCamera handles PATCH /api/config/cameras/:name/toggle — enables/disables a camera.
func (h *Handlers) ToggleCamera(c echo.Context) error {
	h.configEditMu.Lock()
	defer h.configEditMu.Unlock()
	name := c.Param("name")
	cam, ok := h.configSnapshot().Cameras[name]
	if !ok {
		return echo.NewHTTPError(http.StatusNotFound, "camera not found")
	}
	cam.Enabled = !cam.Enabled
	if err := config.SaveCamera(h.db, name, cam); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := h.applyCamera(name, cam); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
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
		go2rtcURL = cameras.FindGo2rtcURL(h.configSnapshot().FrigateURL)
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
			Source:         util.RedactURLString(src),
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
	h.configEditMu.Lock()
	defer h.configEditMu.Unlock()
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

	loaded, err := config.Load(h.db)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	for _, discovered := range cameras {
		if err := h.applyCamera(discovered.Name, loaded.Cameras[discovered.Name]); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	}
	for i := range cameras {
		cameras[i].Pass = ""
		cameras[i].Stream = util.RedactURLString(cameras[i].Stream)
	}

	h.logger(c).Info("cameras discovered via Frigate", "count", len(cameras), "frigate", frigateURL)
	return c.JSON(http.StatusOK, map[string]interface{}{
		"discovered": len(cameras),
		"cameras":    cameras,
	})
}

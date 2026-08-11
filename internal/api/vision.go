package api

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
)

// Snapshot handles GET /api/snapshot/:camera — grabs a JPEG frame.
// By default uses Frigate's latest.jpg (detect stream). If ?stream=<name> is
// provided, uses ffmpeg to grab a frame from that go2rtc RTSP stream instead.
// ?width=<px> optionally scales the frame (ffmpeg only).
func (h *Handlers) Snapshot(c echo.Context) error {
	camera := c.Param("camera")
	if camera == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "camera required")
	}

	streamName := c.QueryParam("stream")
	width := 0
	if w := c.QueryParam("width"); w != "" {
		if _, err := fmt.Sscanf(w, "%d", &width); err != nil || width <= 0 {
			width = 0
		}
	}

	// If a stream is specified, use ffmpeg to grab from go2rtc.
	if streamName != "" {
		if h.cfg.Go2rtcURL == "" {
			return echo.NewHTTPError(http.StatusServiceUnavailable, "go2rtc URL not configured")
		}
		data, err := grabFrameFromStream(h.cfg.Go2rtcURL, streamName, width, 10*time.Second)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadGateway, err.Error())
		}
		c.Response().Header().Set("Content-Type", "image/jpeg")
		c.Response().Header().Set("Cache-Control", "no-cache")
		return c.Blob(http.StatusOK, "image/jpeg", data)
	}

	// Default: Frigate detect stream.
	if h.cfg.FrigateURL == "" {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "frigate URL not configured")
	}

	// ?h=720 forces Frigate to run the frame through its image pipeline (PIL resize),
	// which normalises the JPEG encoding and avoids raw-stream distortion artifacts.
	snapURL := fmt.Sprintf("%s/api/%s/latest.jpg?h=720", h.cfg.FrigateURL, camera)
	req, err := http.NewRequestWithContext(c.Request().Context(), http.MethodGet, snapURL, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return echo.NewHTTPError(
			http.StatusBadGateway,
			fmt.Sprintf("frigate returned HTTP %d", resp.StatusCode),
		)
	}

	c.Response().Header().Set("Content-Type", "image/jpeg")
	c.Response().Header().Set("Cache-Control", "no-cache")
	return c.Stream(http.StatusOK, "image/jpeg", resp.Body)
}

// resolveVisionPrompt picks the first non-empty prompt from the chain:
// request → camera's vision_prompt → global VisionConfig.Prompt.
// If all are empty, returns "" so the vision client uses its hardcoded default.
func resolveVisionPrompt(reqPrompt string, camOk bool, camPrompt, globalPrompt string) string {
	if reqPrompt != "" {
		return reqPrompt
	}
	if camOk && camPrompt != "" {
		return camPrompt
	}
	return globalPrompt
}

// Vision handles POST /api/vision — Frigate snapshot → vision model → description.
// No TTS, no camera send. Useful for cameras without speakers.
func (h *Handlers) Vision(c echo.Context) error {
	log := h.logger(c)

	var req struct {
		Camera string `json:"camera"`
		Prompt string `json:"prompt"`
	}
	if err := c.Bind(&req); err != nil || req.Camera == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "camera required")
	}
	h.cfgMu.Lock()
	frigateURL := h.cfg.FrigateURL
	globalPrompt := h.cfg.Vision.Prompt
	cam, camOk := h.cfg.Cameras[req.Camera]
	visionClient := h.vision
	h.cfgMu.Unlock()

	if visionClient == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "vision model not configured")
	}

	imageBytes, err := h.fetchSnapshot(c.Request().Context(), req.Camera, cam, frigateURL)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}

	prompt := resolveVisionPrompt(req.Prompt, camOk, cam.VisionPrompt, globalPrompt)
	description, err := visionClient.Describe(imageBytes, "image/jpeg", prompt)
	if err != nil {
		log.Error("vision: failed", "camera", req.Camera, "err", err)
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("vision: %s", err))
	}

	log.Info("vision: done", "camera", req.Camera, "text", description)
	h.events.publish(
		event{Camera: req.Camera, Action: "describe", Text: description, At: time.Now()},
	)

	return c.JSON(http.StatusOK, map[string]string{"description": description})
}

// VisionTest handles POST /api/vision/test — captures a snapshot (or reuses
// a client-provided base64 image, or accepts an uploaded image file) and runs
// a vision prompt against it. Returns both the image (base64 data URI) and
// the description, so the UI can display the snapshot and iterate on prompts
// without re-capturing.
//
// Accepts either:
//   - JSON: {camera, prompt, image} where image is a base64 data URI
//   - Multipart form: "prompt" field + "image" file upload
func (h *Handlers) VisionTest(c echo.Context) error {
	log := h.logger(c)
	start := time.Now()
	t := NewStepTimings(2)

	var camera, prompt, imageB64 string

	contentType := c.Request().Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Multipart form upload
		prompt = c.FormValue("prompt")
		file, err := c.FormFile("image")
		if err == nil && file != nil {
			src, err := file.Open()
			if err != nil {
				return echo.NewHTTPError(http.StatusBadRequest, "cannot open uploaded file")
			}
			defer src.Close()
			imgBytes, err := io.ReadAll(src)
			if err != nil {
				return echo.NewHTTPError(http.StatusInternalServerError, "reading uploaded file")
			}
			mimeType := file.Header.Get("Content-Type")
			if mimeType == "" {
				mimeType = http.DetectContentType(imgBytes)
			}
			imageB64 = "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(imgBytes)
		}
	} else {
		// JSON body
		var req struct {
			Camera string `json:"camera"`
			Prompt string `json:"prompt"`
			Image  string `json:"image"`
		}
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		camera = req.Camera
		prompt = req.Prompt
		imageB64 = req.Image
	}

	h.cfgMu.Lock()
	frigateURL := h.cfg.FrigateURL
	visionClient := h.vision
	h.cfgMu.Unlock()

	if visionClient == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "vision model not configured")
	}

	var imageBytes []byte
	var imageDataURI string

	if imageB64 != "" {
		// Client provided an image (uploaded or cached) — decode and reuse
		b64Data := imageB64
		if idx := strings.IndexByte(b64Data, ','); idx > 0 && len(b64Data) > 20 &&
			b64Data[:5] == "data:" {
			b64Data = b64Data[idx+1:]
		}
		decoded, err := base64.StdEncoding.DecodeString(b64Data)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid base64 image")
		}
		imageBytes = decoded
		imageDataURI = imageB64
	} else {
		// Capture a fresh snapshot (from configured vision_stream or Frigate)
		if camera == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "camera required (or provide image)")
		}
		h.cfgMu.Lock()
		camCfg := h.cfg.Cameras[camera]
		h.cfgMu.Unlock()

		snapStart := time.Now()
		var snapErr error
		imageBytes, snapErr = h.fetchSnapshot(c.Request().Context(), camera, camCfg, frigateURL)
		if snapErr != nil {
			return echo.NewHTTPError(http.StatusBadGateway, snapErr.Error())
		}
		t.Add("snapshot_ms", snapStart)
		imageDataURI = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(imageBytes)
	}

	visionStart := time.Now()
	description, err := visionClient.Describe(imageBytes, "image/jpeg", prompt)
	if err != nil {
		log.Error("vision-test: failed", "camera", camera, "err", err)
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("vision: %s", err))
	}
	t.Add("vision_ms", visionStart)

	log.Info(
		"vision-test: done",
		"camera",
		camera,
		"prompt_len",
		len(prompt),
		"text",
		description,
		"elapsed",
		time.Since(start),
	)

	return c.JSON(http.StatusOK, map[string]any{
		"description": description,
		"image":       imageDataURI,
		"timings":     t.Ms(),
		"ttfs_ms":     t.TTFS(),
		"total_ms":    TotalMs(start),
	})
}

// Describe handles POST /api/describe — Frigate snapshot → vision model → TTS → camera.
func (h *Handlers) Describe(c echo.Context) error {
	log := h.logger(c)

	var req struct {
		Camera string  `json:"camera"`
		Prompt string  `json:"prompt"`
		Gain   float64 `json:"gain"`
	}
	if err := c.Bind(&req); err != nil || req.Camera == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "camera required")
	}

	h.cfgMu.Lock()
	frigateURL := h.cfg.FrigateURL
	globalPrompt := h.cfg.Vision.Prompt
	camCfg, camOk := h.cfg.Cameras[req.Camera]
	visionClient := h.vision
	defaultVoice := h.cfg.TTS.DefaultVoice
	h.cfgMu.Unlock()

	if visionClient == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "vision model not configured")
	}

	start := time.Now()
	t := NewStepTimings(4)
	log.Info("describe: request", "camera", req.Camera)

	// 1. Fetch snapshot (from configured vision_stream or Frigate)
	snapStart := time.Now()
	imageBytes, err := h.fetchSnapshot(c.Request().Context(), req.Camera, camCfg, frigateURL)
	if err != nil {
		log.Error("describe: snapshot failed", "camera", req.Camera, "err", err)
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	t.Add("snap_ms", snapStart)
	log.Debug(
		"describe: snapshot fetched",
		"camera",
		req.Camera,
		"bytes",
		len(imageBytes),
		"elapsed",
		time.Since(snapStart),
	)
	t.Add("snapshot_ms", snapStart)

	// 2. Send to vision model (resolve prompt: request → camera → global → default)
	prompt := resolveVisionPrompt(req.Prompt, camOk, camCfg.VisionPrompt, globalPrompt)
	visionStart := time.Now()
	description, err := visionClient.Describe(imageBytes, "image/jpeg", prompt)
	if err != nil {
		log.Error("describe: vision failed", "camera", req.Camera, "err", err)
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("vision: %s", err))
	}
	log.Info(
		"describe: vision result",
		"camera",
		req.Camera,
		"text",
		description,
		"elapsed",
		time.Since(visionStart),
	)
	t.Add("vision_ms", visionStart)

	// 3. TTS
	voice := defaultVoice
	ttsStart := time.Now()
	wav, err := h.tts.Speak(description, voice)
	if err != nil {
		log.Error("describe: TTS failed", "camera", req.Camera, "err", err)
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("TTS: %s", err))
	}
	log.Debug(
		"describe: TTS generated",
		"camera",
		req.Camera,
		"wav_bytes",
		len(wav),
		"elapsed",
		time.Since(ttsStart),
	)
	t.Add("tts_ms", ttsStart)

	// 4. Transcode + send to camera
	gain := req.Gain
	if gain <= 0 {
		gain = 3.0
	}

	transcodeStart := time.Now()
	rawPath, err := wavBytesToRaw(wav, h.tmpDir, gain)
	if err != nil {
		return echo.NewHTTPError(
			http.StatusInternalServerError,
			fmt.Sprintf("transcoding: %s", err),
		)
	}
	t.Add("transcode_ms", transcodeStart)
	defer os.Remove(rawPath)

	cam, err := h.reg.Get(req.Camera)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	sendTiming, err := cam.SendRaw(rawPath)
	if err != nil {
		log.Error("describe: send failed", "camera", req.Camera, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	t.steps["send_open_ms"] = time.Duration(sendTiming.OpenMs) * time.Millisecond
	t.steps["send_playback_ms"] = time.Duration(sendTiming.PlaybackMs) * time.Millisecond
	log.Debug(
		"describe: camera send complete",
		"camera",
		req.Camera,
		"open_ms",
		sendTiming.OpenMs,
		"playback_ms",
		sendTiming.PlaybackMs,
	)

	log.Info("describe: done", "camera", req.Camera, "elapsed", time.Since(start), "ttfs_ms", t.TTFS())
	h.events.publish(
		event{Camera: req.Camera, Action: "describe", Text: description, At: time.Now()},
	)

	snapB64 := base64.StdEncoding.EncodeToString(imageBytes)
	return c.JSON(http.StatusOK, map[string]any{
		"status":      "ok",
		"description": description,
		"image":       "data:image/jpeg;base64," + snapB64,
		"timings":     t.Ms(),
		"ttfs_ms":     t.TTFS(),
		"total_ms":    TotalMs(start),
	})
}

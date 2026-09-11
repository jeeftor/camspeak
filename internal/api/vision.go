package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	clog "github.com/charmbracelet/log"
	"github.com/labstack/echo/v4"

	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/vision"
)

// Snapshot handles GET /api/snapshot/:camera — grabs a JPEG frame.
// For Hikvision cameras, tries the direct ISAPI snapshot first (fastest).
// If ?stream=<name> is provided, uses ffmpeg to grab a frame from that go2rtc
// RTSP stream instead. ?width=<px> optionally scales the frame (ffmpeg only).
func (h *Handlers) Snapshot(c echo.Context) error {
	cfg := h.configSnapshot()
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

	// Use the shared fetchSnapshot (tries ISAPI for Hikvision, then go2rtc, then Frigate).
	camCfg := cfg.Cameras[camera]
	if method := c.QueryParam("method"); method != "" {
		camCfg.SnapMethod = method
	} else if streamName != "" && streamName != "main" && streamName != "sub" {
		camCfg.SnapMethod = "go2rtc"
	}
	if c.QueryParam("width") != "" {
		camCfg.VisionWidth = width
	}
	frigateURL := cfg.FrigateURL

	data, source, err := h.fetchSnapshotSource(
		c.Request().Context(),
		camera,
		camCfg,
		frigateURL,
		streamName,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	c.Response().Header().Set("Content-Type", "image/jpeg")
	c.Response().Header().Set("X-Capture-Source", source)
	c.Response().Header().Set("Cache-Control", "no-cache")
	return c.Blob(http.StatusOK, "image/jpeg", data)
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
	cfg := h.configSnapshot()
	log := h.logger(c)

	var req struct {
		Camera string `json:"camera"`
		Stream string `json:"stream"`
		Prompt string `json:"prompt"`
	}
	if err := c.Bind(&req); err != nil || req.Camera == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "camera required")
	}
	frigateURL := cfg.FrigateURL
	globalPrompt := cfg.Vision.Prompt
	cam, camOk := cfg.Cameras[req.Camera]
	visionClient := h.visionClient()

	if visionClient == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "vision model not configured")
	}

	imageBytes, err := h.fetchSnapshot(
		c.Request().Context(),
		req.Camera,
		cam,
		frigateURL,
		req.Stream,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}

	prompt := resolveVisionPrompt(req.Prompt, camOk, cam.VisionPrompt, globalPrompt)
	description, err := visionClient.DescribeContext(
		c.Request().Context(),
		imageBytes,
		"image/jpeg",
		prompt,
	)
	if err != nil {
		log.Error("vision: failed", "camera", req.Camera, "err", err)
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("vision: %s", err))
	}

	log.Info("vision: done", "camera", req.Camera, "text", description)
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
	cfg := h.configSnapshot()
	log := h.logger(c)
	start := time.Now()
	t := NewStepTimings(2)

	var camera, prompt, imageB64, streamOverride string
	var visionClient *vision.Client

	contentType := c.Request().Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Multipart form upload
		prompt = c.FormValue("prompt")
		streamOverride = c.FormValue("stream")
		modelOverride := c.FormValue("model")
		if modelOverride != "" {
			url := cfg.Vision.URL
			apiKey := cfg.Vision.APIKey
			if url != "" {
				visionClient = vision.NewClient(url, modelOverride, apiKey)
			}
		}
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
			Stream string `json:"stream"`
			Prompt string `json:"prompt"`
			Image  string `json:"image"`
			Model  string `json:"model"`
		}
		if err := c.Bind(&req); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
		}
		camera = req.Camera
		prompt = req.Prompt
		imageB64 = req.Image
		streamOverride = req.Stream
		// Use the per-request model override if provided; otherwise fall
		// back to the globally configured model (handled by Describe below).
		if req.Model != "" {
			url := cfg.Vision.URL
			apiKey := cfg.Vision.APIKey
			if url != "" {
				visionClient = vision.NewClient(url, req.Model, apiKey)
			}
		}
	}
	frigateURL := cfg.FrigateURL
	if visionClient == nil {
		visionClient = h.visionClient()
	}

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
		camCfg := cfg.Cameras[camera]

		snapStart := time.Now()
		var snapErr error
		imageBytes, snapErr = h.fetchSnapshot(c.Request().Context(), camera, camCfg, frigateURL, streamOverride)
		if snapErr != nil {
			return echo.NewHTTPError(http.StatusBadGateway, snapErr.Error())
		}
		t.Add("snapshot_ms", snapStart)
		imageDataURI = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(imageBytes)
	}

	visionStart := time.Now()
	description, err := visionClient.DescribeContext(
		c.Request().Context(),
		imageBytes,
		"image/jpeg",
		prompt,
	)
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
		"model":       visionClient.Model(),
		"timings":     t.Ms(),
		"ttfs_ms":     t.TTFS(),
		"total_ms":    TotalMs(start),
	})
}

type describeRequest struct {
	Camera string   `json:"camera"`
	Stream string   `json:"stream"`
	Prompt string   `json:"prompt"`
	Gain   *float64 `json:"gain"`
}

// Describe handles POST /api/describe — snapshot, vision model, TTS, then camera.
func (h *Handlers) Describe(c echo.Context) error {
	cfg := h.configSnapshot()
	log := h.logger(c)

	var req describeRequest
	if err := c.Bind(&req); err != nil || req.Camera == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "camera required")
	}
	if err := validateRequestGain(req.Gain); err != nil {
		return err
	}
	visionClient := h.visionClient()

	if visionClient == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "vision model not configured")
	}
	cam, err := h.reg.GetForPlayback(req.Camera)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	op, err := h.beginPlaybackOperation(
		c.Request().Context(),
		req.Camera,
		cam,
		"describe",
		"vision description",
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	}
	defer op.finish()
	result, err := h.runDescribe(op, req, cfg, visionClient, log, nil)
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, result)
}

// runDescribe is shared by synchronous clients and the WAN-safe background job.
func (h *Handlers) runDescribe(
	op *playbackOperation, req describeRequest, cfg config.Config,
	visionClient *vision.Client, log *clog.Logger,
	progress func(string, map[string]any),
) (map[string]any, error) {
	start := time.Now()
	t := NewStepTimings(4)
	result := map[string]any{}
	result["tts_mode"] = "buffered"
	if canStreamSpeech(op, cfg) {
		result["tts_mode"] = "streaming"
	}
	report := func(stage string) {
		result["capture_source"] = t.CaptureSource
		result["timings"] = t.Ms()
		result["total_ms"] = TotalMs(start)
		if progress != nil {
			progress(stage, result)
		}
	}
	t.onStep = func(name string) {
		switch name {
		case "snapshot_ms":
			report("vision")
		case "vision_ms":
			report("tts")
		}
	}
	report("snapshot")
	log.Info("describe: request", "camera", req.Camera)

	camCfg, camOK := cfg.Cameras[req.Camera]
	prompt := resolveVisionPrompt(req.Prompt, camOK, camCfg.VisionPrompt, cfg.Vision.Prompt)
	imageBytes, description, err := h.describeImage(
		op.ctx,
		req.Camera,
		camCfg,
		cfg.FrigateURL,
		req.Stream,
		prompt,
		visionClient,
		t,
	)
	if err != nil {
		log.Error("describe: vision failed", "camera", req.Camera, "err", err)
		return result, echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("vision: %s", err))
	}
	result["description"] = description
	log.Info("describe: vision result", "camera", req.Camera, "model", cfg.Vision.Model,
		"text", description, "text_bytes", len(description))
	report("tts")

	// 3. TTS
	voice := cfg.TTS.DefaultVoice
	if canStreamSpeech(op, cfg) {
		op.detail = description
		err = h.streamSpeech(
			op,
			cfg,
			description,
			voice,
			h.gainForCall(req.Camera, requestGain(req.Gain)),
			t,
			func(stage string) {
				if stage == "playing" {
					result["ttfs_ms"] = t.TTFS()
				}
				report(stage)
			},
		)
		if err != nil {
			log.Error("describe: streaming failed", "camera", req.Camera, "err", err)
			return result, echo.NewHTTPError(http.StatusBadGateway, err.Error())
		}
	} else {
		ttsStart := time.Now()
		wav, err := h.ttsClient().SpeakContext(op.ctx, description, voice)
		if err != nil {
			log.Error("describe: TTS failed", "camera", req.Camera, "err", err)
			return result, echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("TTS: %s", err))
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
		report("transcode")

		// 4. Transcode + send to camera
		// Gain is applied at send time via GainController (per-chunk).

		transcodeStart := time.Now()
		rawPath, err := wavBytesToRawWithPrimeContext(op.ctx, wav, h.tmpDir, 1.0, cfg.PrimeSilenceMs)
		if err != nil {
			return result, echo.NewHTTPError(
				http.StatusInternalServerError,
				fmt.Sprintf("transcoding: %s", err),
			)
		}
		t.Add("transcode_ms", transcodeStart)
		defer os.Remove(rawPath)

		op.detail = description
		report("connecting")
		sendStart := time.Now()
		op.onPlaying = func() {
			t.Add("send_open_ms", sendStart)
			result["ttfs_ms"] = t.TTFS()
			report("playing")
		}
		sendTiming, err := op.send(rawPath, h.gainForCall(req.Camera, requestGain(req.Gain)))
		if err != nil {
			log.Error("describe: send failed", "camera", req.Camera, "err", err)
			return result, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if _, reported := t.steps["send_open_ms"]; !reported {
			t.steps["send_open_ms"] = time.Duration(sendTiming.OpenMs) * time.Millisecond
		}
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

	}
	log.Info(
		"describe: done",
		"camera",
		req.Camera,
		"elapsed",
		time.Since(start),
		"ttfs_ms",
		t.TTFS(),
	)
	h.events.publish(
		event{
			Camera: req.Camera, Action: "describe", Text: description, At: time.Now(),
			Replay: playbackReplay(
				"/api/describe",
				map[string]any{"camera": req.Camera, "stream": req.Stream, "prompt": req.Prompt},
				requestGain(req.Gain),
			),
		},
	)

	snapB64 := base64.StdEncoding.EncodeToString(imageBytes)
	result["status"] = "ok"
	result["image"] = "data:image/jpeg;base64," + snapB64
	result["ttfs_ms"] = t.TTFS()
	report("done")
	return result, nil
}

// Announce handles POST /api/announce — captures a snapshot from a source
// camera, runs vision to generate a description, then TTS-plays that
// description on a target camera's speaker. This enables cross-camera
// scenarios like "capture from doorbell, announce on frontyard speaker".
func (h *Handlers) Announce(c echo.Context) error {
	cfg := h.configSnapshot()
	log := h.logger(c)

	var req struct {
		SourceCamera string   `json:"source_camera"`
		TargetCamera string   `json:"target_camera"`
		Stream       string   `json:"stream"`
		Prompt       string   `json:"prompt"`
		Voice        string   `json:"voice"`
		Gain         *float64 `json:"gain"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	if err := validateRequestGain(req.Gain); err != nil {
		return err
	}
	if req.SourceCamera == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "source_camera required")
	}
	if req.TargetCamera == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "target_camera required")
	}
	frigateURL := cfg.FrigateURL
	globalPrompt := cfg.Vision.Prompt
	srcCfg, srcOk := cfg.Cameras[req.SourceCamera]
	defaultVoice := cfg.TTS.DefaultVoice

	visionClient := h.visionClient()
	if visionClient == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "vision model not configured")
	}
	cam, err := h.reg.GetForPlayback(req.TargetCamera)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	op, err := h.beginPlaybackOperation(
		c.Request().Context(),
		req.TargetCamera,
		cam,
		"announce",
		"vision description",
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	}
	defer op.finish()

	start := time.Now()
	t := NewStepTimings(4)
	log.Info("announce: request", "source", req.SourceCamera, "target", req.TargetCamera)

	prompt := resolveVisionPrompt(req.Prompt, srcOk, srcCfg.VisionPrompt, globalPrompt)
	imageBytes, description, err := h.describeImage(
		op.ctx,
		req.SourceCamera,
		srcCfg,
		frigateURL,
		req.Stream,
		prompt,
		visionClient,
		t,
	)
	if err != nil {
		log.Error("announce: vision failed", "source", req.SourceCamera, "err", err)
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("vision: %s", err))
	}
	log.Info("announce: vision result", "source", req.SourceCamera, "text", description)

	// 3. TTS
	voice := req.Voice
	if voice == "" {
		voice = defaultVoice
	}
	ttsStart := time.Now()
	wav, err := h.ttsClient().SpeakContext(op.ctx, description, voice)
	if err != nil {
		log.Error("announce: TTS failed", "err", err)
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("TTS: %s", err))
	}
	t.Add("tts_ms", ttsStart)

	// 4. Transcode + send to target camera
	transcodeStart := time.Now()
	rawPath, err := wavBytesToRawWithPrimeContext(op.ctx, wav, h.tmpDir, 1.0, cfg.PrimeSilenceMs)
	if err != nil {
		return echo.NewHTTPError(
			http.StatusInternalServerError,
			fmt.Sprintf("transcoding: %s", err),
		)
	}
	t.Add("transcode_ms", transcodeStart)
	defer os.Remove(rawPath)

	op.detail = description
	sendTiming, err := op.send(rawPath, h.gainForCall(req.TargetCamera, requestGain(req.Gain)))
	if err != nil {
		log.Error("announce: send failed", "target", req.TargetCamera, "err", err)
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	t.steps["send_open_ms"] = time.Duration(sendTiming.OpenMs) * time.Millisecond
	t.steps["send_playback_ms"] = time.Duration(sendTiming.PlaybackMs) * time.Millisecond

	log.Info(
		"announce: done",
		"source",
		req.SourceCamera,
		"target",
		req.TargetCamera,
		"elapsed",
		time.Since(start),
	)
	h.events.publish(event{
		Camera: req.TargetCamera, Action: "announce", Text: description, At: time.Now(),
		Replay: playbackReplay(
			"/api/announce",
			map[string]any{
				"source_camera": req.SourceCamera,
				"target_camera": req.TargetCamera,
				"stream":        req.Stream,
				"prompt":        req.Prompt,
				"voice":         req.Voice,
			},
			requestGain(req.Gain),
		),
	})

	return c.JSON(http.StatusOK, map[string]any{
		"status":        "ok",
		"description":   description,
		"source_camera": req.SourceCamera,
		"target_camera": req.TargetCamera,
		"image":         "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(imageBytes),
		"timings":       t.Ms(),
		"ttfs_ms":       t.TTFS(),
		"total_ms":      TotalMs(start),
	})
}

// describeImage shares the exact capture and inference pipeline for descriptions
// and cross-camera announcements. The returned image is the inference input.
func (h *Handlers) describeImage(
	ctx context.Context,
	camera string,
	cfg config.CameraConfig,
	frigateURL, stream, prompt string,
	client *vision.Client,
	timings *StepTimings,
) ([]byte, string, error) {
	start := time.Now()
	image, source, err := h.fetchSnapshotSource(ctx, camera, cfg, frigateURL, stream)
	timings.CaptureSource = source
	if err != nil {
		return nil, "", fmt.Errorf("snapshot: %w", err)
	}
	timings.Add("snap_ms", start)
	timings.Add("snapshot_ms", start)
	start = time.Now()
	text, err := client.DescribeContext(ctx, image, "image/jpeg", prompt)
	if err != nil {
		return nil, "", fmt.Errorf("vision: %w", err)
	}
	timings.Add("vision_ms", start)
	return image, text, nil
}

func isVisionCapableModel(id string) bool {
	lower := strings.ToLower(id)
	// Substring keywords that unambiguously indicate vision capability.
	for _, kw := range []string{"vision", "llava", "moondream", "cogvlm", "internvl", "pixtral"} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	// Match "-VL-", "-VL_", "-VL." separators or "-VL" / "_VL" at end of name.
	for _, sep := range []string{"-vl-", "-vl_", "-vl.", "_vl-", "_vl_"} {
		if strings.Contains(lower, sep) {
			return true
		}
	}
	if strings.HasSuffix(lower, "-vl") || strings.HasSuffix(lower, "_vl") {
		return true
	}
	// MiniCPM-V series (e.g. "MiniCPM-V-4.6-GGUF").
	if strings.Contains(lower, "minicpm-v") || strings.Contains(lower, "minicpm_v") {
		return true
	}
	return false
}

// visionModelResult is a single model's result from VisionTestAll.
type visionModelResult struct {
	Model       string `json:"model"`
	Description string `json:"description,omitempty"`
	Error       string `json:"error,omitempty"`
	TtfsMs      int64  `json:"ttfs_ms"`  // load + prefill (time to first token)
	GenMs       int64  `json:"gen_ms"`   // token generation time
	TotalMs     int64  `json:"total_ms"` // total wall-clock
}

// VisionTestAll handles POST /api/vision/test-all — runs the same image/prompt
// against every model returned by /v1/models on the configured vision endpoint,
// in parallel. Returns the image (base64 data URI) and per-model results.
func (h *Handlers) VisionTestAll(c echo.Context) error {
	cfg := h.configSnapshot()
	log := h.logger(c)

	var req struct {
		Camera string `json:"camera"`
		Stream string `json:"stream"`
		Prompt string `json:"prompt"`
		Image  string `json:"image"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	frigateURL := cfg.FrigateURL
	visionClient := h.visionClient()

	if visionClient == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "vision model not configured")
	}

	// Resolve image bytes.
	var imageBytes []byte
	var imageDataURI string

	if req.Image != "" {
		b64Data := req.Image
		if idx := strings.IndexByte(b64Data, ','); idx > 0 && len(b64Data) > 5 &&
			b64Data[:5] == "data:" {
			b64Data = b64Data[idx+1:]
		}
		decoded, err := base64.StdEncoding.DecodeString(b64Data)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid base64 image")
		}
		imageBytes = decoded
		imageDataURI = req.Image
	} else {
		if req.Camera == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "camera required (or provide image)")
		}
		camCfg := cfg.Cameras[req.Camera]

		var snapErr error
		imageBytes, snapErr = h.fetchSnapshot(c.Request().Context(), req.Camera, camCfg, frigateURL, req.Stream)
		if snapErr != nil {
			return echo.NewHTTPError(http.StatusBadGateway, snapErr.Error())
		}
		imageDataURI = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(imageBytes)
	}

	// Fetch available models from the vision endpoint.
	modelsURL := visionClient.URL()
	if idx := strings.Index(modelsURL, "/v1/"); idx >= 0 {
		modelsURL = modelsURL[:idx+4] + "models"
	} else {
		modelsURL = strings.TrimRight(modelsURL, "/") + "/v1/models"
	}

	httpReq, err := http.NewRequestWithContext(
		c.Request().Context(),
		http.MethodGet,
		modelsURL,
		nil,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if key := visionClient.APIKey(); key != "" {
		httpReq.Header.Set("Authorization", "Bearer "+key)
	}
	modelsResp, err := (&http.Client{Timeout: 10 * time.Second}).Do(httpReq)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("fetching models: %s", err))
	}
	defer modelsResp.Body.Close()

	var modelsData struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(modelsResp.Body).Decode(&modelsData); err != nil ||
		len(modelsData.Data) == 0 {
		return echo.NewHTTPError(http.StatusBadGateway, "no models returned from vision endpoint")
	}

	allModels := make([]string, 0, len(modelsData.Data))
	for _, m := range modelsData.Data {
		if m.ID != "" {
			allModels = append(allModels, m.ID)
		}
	}
	models := make([]string, 0, len(allModels))
	for _, m := range allModels {
		if isVisionCapableModel(m) {
			models = append(models, m)
		}
	}
	log.Info(
		"vision-test-all: running",
		"total_models",
		len(allModels),
		"vision_models",
		len(models),
		"prompt_len",
		len(req.Prompt),
	)
	if len(models) == 0 {
		return echo.NewHTTPError(
			http.StatusBadGateway,
			"no vision-capable models found at endpoint",
		)
	}

	// Run models sequentially — they share a GPU, parallel would thrash VRAM.
	results := make([]visionModelResult, 0, len(models))
	for _, model := range models {
		log.Info("vision-test-all: testing model", "model", model)
		desc, timing, err := visionClient.DescribeWithModelTimedContext(c.Request().Context(),
			imageBytes,
			"image/jpeg",
			req.Prompt,
			model,
		)
		r := visionModelResult{
			Model:   model,
			TtfsMs:  timing.TtfsMs,
			GenMs:   timing.GenMs,
			TotalMs: timing.TotalMs,
		}
		if err != nil {
			r.Error = err.Error()
		} else {
			r.Description = desc
		}
		results = append(results, r)
	}

	log.Info("vision-test-all: done", "models", len(models))
	return c.JSON(http.StatusOK, map[string]any{
		"image":   imageDataURI,
		"results": results,
	})
}

// VisionTestAllStream handles POST /api/vision/test-all/stream — same as VisionTestAll
// but uses SSE to stream results as each model completes, so the UI can update live.
//
// SSE events:
//
//	{"type":"image","image":"data:image/jpeg;base64,..."}  — captured image (first)
//	{"type":"models","models":["model1","model2",...]}      — full model list
//	{"type":"result","model":"m","description":"...","ttfs_ms":N,"gen_ms":N,"total_ms":N,"error":"..."}
//	{"type":"done","count":N}
func (h *Handlers) VisionTestAllStream(c echo.Context) error {
	cfg := h.configSnapshot()
	log := h.logger(c)

	var req struct {
		Camera string `json:"camera"`
		Stream string `json:"stream"`
		Prompt string `json:"prompt"`
		Image  string `json:"image"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}
	frigateURL := cfg.FrigateURL
	visionClient := h.visionClient()

	if visionClient == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "vision model not configured")
	}

	// Resolve image bytes.
	var imageBytes []byte
	var imageDataURI string

	if req.Image != "" {
		b64Data := req.Image
		if idx := strings.IndexByte(b64Data, ','); idx > 0 && len(b64Data) > 5 &&
			b64Data[:5] == "data:" {
			b64Data = b64Data[idx+1:]
		}
		decoded, err := base64.StdEncoding.DecodeString(b64Data)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "invalid base64 image")
		}
		imageBytes = decoded
		imageDataURI = req.Image
	} else {
		if req.Camera == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "camera required (or provide image)")
		}
		camCfg := cfg.Cameras[req.Camera]

		var snapErr error
		imageBytes, snapErr = h.fetchSnapshot(c.Request().Context(), req.Camera, camCfg, frigateURL, req.Stream)
		if snapErr != nil {
			return echo.NewHTTPError(http.StatusBadGateway, snapErr.Error())
		}
		imageDataURI = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(imageBytes)
	}

	// Fetch available models.
	modelsURL := visionClient.URL()
	if idx := strings.Index(modelsURL, "/v1/"); idx >= 0 {
		modelsURL = modelsURL[:idx+4] + "models"
	} else {
		modelsURL = strings.TrimRight(modelsURL, "/") + "/v1/models"
	}
	httpReq, err := http.NewRequestWithContext(
		c.Request().Context(),
		http.MethodGet,
		modelsURL,
		nil,
	)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if key := visionClient.APIKey(); key != "" {
		httpReq.Header.Set("Authorization", "Bearer "+key)
	}
	modelsResp, err := (&http.Client{Timeout: 10 * time.Second}).Do(httpReq)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("fetching models: %s", err))
	}
	defer modelsResp.Body.Close()

	var modelsData struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(modelsResp.Body).Decode(&modelsData); err != nil ||
		len(modelsData.Data) == 0 {
		return echo.NewHTTPError(http.StatusBadGateway, "no models returned from vision endpoint")
	}
	allModels := make([]string, 0, len(modelsData.Data))
	for _, m := range modelsData.Data {
		if m.ID != "" {
			allModels = append(allModels, m.ID)
		}
	}
	models := make([]string, 0, len(allModels))
	for _, m := range allModels {
		if isVisionCapableModel(m) {
			models = append(models, m)
		}
	}
	log.Info(
		"vision-test-all-stream: filtered models",
		"total",
		len(allModels),
		"vision",
		len(models),
	)
	if len(models) == 0 {
		return echo.NewHTTPError(
			http.StatusBadGateway,
			"no vision-capable models found at endpoint",
		)
	}

	// Switch to SSE.
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("X-Accel-Buffering", "no")
	c.Response().WriteHeader(http.StatusOK)

	sseWrite := func(v any) {
		data, _ := json.Marshal(v)
		fmt.Fprintf(c.Response(), "data: %s\n\n", data)
		c.Response().Flush()
	}

	// Send the captured image so the frontend can display it immediately.
	sseWrite(map[string]any{"type": "image", "image": imageDataURI})

	// Send model list so UI can pre-populate cards.
	sseWrite(map[string]any{"type": "models", "models": models})

	log.Info("vision-test-all-stream: running", "models", len(models))

	// Run models sequentially — they share a GPU.
	for _, model := range models {
		if err := c.Request().Context().Err(); err != nil {
			log.Info("vision-test-all-stream: canceled", "reason", err)
			return nil
		}
		log.Info("vision-test-all-stream: testing model", "model", model)
		desc, timing, err := visionClient.DescribeWithModelTimedContext(c.Request().Context(),
			imageBytes,
			"image/jpeg",
			req.Prompt,
			model,
		)
		if err := c.Request().Context().Err(); err != nil {
			log.Info("vision-test-all-stream: canceled", "model", model, "reason", err)
			return nil
		}
		r := map[string]any{
			"type":     "result",
			"model":    model,
			"ttfs_ms":  timing.TtfsMs,
			"gen_ms":   timing.GenMs,
			"total_ms": timing.TotalMs,
		}
		if err != nil {
			r["error"] = err.Error()
		} else {
			r["description"] = desc
		}
		sseWrite(r)
	}

	sseWrite(map[string]any{"type": "done", "count": len(models)})
	log.Info("vision-test-all-stream: done", "models", len(models))
	return nil
}

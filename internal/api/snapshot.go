package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/vision"
)

// grabFrameFromStream captures a single frame from a go2rtc stream.
// It first tries go2rtc's native frame.jpeg endpoint (fast, ~500ms), then
// falls back to ffmpeg if that fails (e.g. H265 streams that go2rtc can't
// decode internally). If maxWidth > 0, the frame is scaled via ffmpeg -vf
// (only applies to the ffmpeg path; go2rtc's frame.jpeg returns native res).
func grabFrameFromStream(
	go2rtcURL, streamName string,
	maxWidth int,
	timeout time.Duration,
) ([]byte, error) {
	// Try go2rtc's native frame.jpeg first (much faster — no ffmpeg startup).
	if data, err := grabFrameViaGo2rtcAPI(go2rtcURL, streamName, timeout); err == nil {
		return data, nil
	}
	// Fall back to ffmpeg (works for all codecs, but slower).
	return grabFrameViaFFmpeg(go2rtcURL, streamName, maxWidth, timeout)
}

// grabFrameViaGo2rtcAPI uses go2rtc's built-in /api/frame.jpeg endpoint.
// This is fast (~500ms) but only works for codecs go2rtc can decode (H264,
// some H265). Does not support width scaling.
func grabFrameViaGo2rtcAPI(go2rtcURL, streamName string, timeout time.Duration) ([]byte, error) {
	frameURL := strings.TrimSuffix(
		go2rtcURL,
		"/",
	) + "/api/frame.jpeg?src=" + url.QueryEscape(
		streamName,
	)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, frameURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("go2rtc frame.jpeg returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(data) < 100 {
		return nil, fmt.Errorf("go2rtc frame.jpeg returned too little data (%d bytes)", len(data))
	}
	return data, nil
}

// grabFrameViaFFmpeg uses ffmpeg to capture a single frame from a go2rtc RTSP
// stream. Works for all codecs (including H265 main streams). If maxWidth > 0,
// the frame is scaled to fit within that width (preserving aspect ratio).
func grabFrameViaFFmpeg(
	go2rtcURL, streamName string,
	maxWidth int,
	timeout time.Duration,
) ([]byte, error) {
	// go2rtc exposes RTSP on port 8554, but the API URL might point to port 1984.
	// We need to derive the RTSP URL from the go2rtc URL's host.
	rtspHost := strings.Replace(go2rtcURL, "http://", "", 1)
	rtspHost = strings.Replace(rtspHost, "https://", "", 1)
	// Strip any path
	if idx := strings.Index(rtspHost, "/"); idx >= 0 {
		rtspHost = rtspHost[:idx]
	}
	// go2rtc's RTSP listener is on port 8554, not the API port.
	// Replace the API port with 8554.
	if idx := strings.LastIndex(rtspHost, ":"); idx >= 0 {
		rtspHost = rtspHost[:idx]
	}
	rtspURL := fmt.Sprintf("rtsp://%s:8554/%s", rtspHost, streamName)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	tmpFile, err := os.CreateTemp("", "camspeak-snap-*.jpg")
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	tmpFile.Close()

	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-rtsp_transport", "tcp",
		"-i", rtspURL,
	}
	if maxWidth > 0 {
		args = append(args, "-vf", fmt.Sprintf("scale=%d:-1", maxWidth))
	}
	args = append(args, "-frames:v", "1", tmpPath)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg frame grab failed: %w (output: %s)", err, string(output))
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("reading captured frame: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("ffmpeg produced empty output")
	}
	return data, nil
}

// grabRTSPFrame captures a single JPEG frame from a direct RTSP URL using ffmpeg.
// This is used for Hikvision main/sub streams where the ISAPI /picture endpoint
// may return the wrong resolution (some cameras always return the main stream
// resolution regardless of the channel number). The RTSP stream delivers the
// actual configured resolution for each channel.
func grabRTSPFrame(rtspURL string, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	tmpFile, err := os.CreateTemp("", "camspeak-rtsp-*.jpg")
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	tmpFile.Close()

	args := []string{
		"-y",
		"-rtsp_transport", "tcp",
		"-i", rtspURL,
		"-frames:v", "1",
		"-update", "1",
		tmpPath,
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("ffmpeg rtsp frame grab failed: %w (output: %s)", err, string(output))
	}

	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("reading captured frame: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("ffmpeg produced empty output")
	}
	return data, nil
}

// BenchmarkResult is a single method's benchmark result.
type BenchmarkResult struct {
	Method    string  `json:"method"`
	OK        bool    `json:"ok"`
	SnapSec   float64 `json:"snap_sec"`
	VisionSec float64 `json:"vision_sec,omitempty"`
	TotalSec  float64 `json:"total_sec,omitempty"`
	Bytes     int     `json:"bytes"`
	Width     int     `json:"width,omitempty"`
	Height    int     `json:"height,omitempty"`
	Model     string  `json:"model,omitempty"`
	Preview   string  `json:"preview,omitempty"`
	Image     string  `json:"image,omitempty"`
	Error     string  `json:"error,omitempty"`
}

// decodeJPEGDimensions reads the SOF0/SOF2 marker from a JPEG to extract
// width and height without fully decoding the image. This is called AFTER
// timing is complete so it doesn't affect snap_sec or vision_sec.
func decodeJPEGDimensions(data []byte) (w, h int) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 0, 0
	}
	// Scan markers starting at offset 2
	i := 2
	for i < len(data)-1 {
		if data[i] != 0xFF {
			i++
			continue
		}
		marker := data[i+1]
		// SOF0 (0xC0) through SOF15 (0xCF), excluding SOF4-SOF7 and SOF8-SOF11
		if marker >= 0xC0 && marker <= 0xCF && marker != 0xC4 && marker != 0xC8 && marker != 0xCC {
			if i+9 < len(data) {
				h = int(data[i+5])<<8 | int(data[i+6])
				w = int(data[i+7])<<8 | int(data[i+8])
				return w, h
			}
		}
		// Skip this marker's payload
		if i+3 < len(data) {
			segLen := int(data[i+2])<<8 | int(data[i+3])
			i += 2 + segLen
		} else {
			break
		}
	}
	return 0, 0
}

// runCameraBenchmark runs all available snapshot methods for a single camera
// and returns results. If withVision is true, also runs the vision model.
// promptOverride, if non-empty, replaces the camera's configured prompt.
// onResult, if non-nil, is called after each method completes (for streaming).
func (h *Handlers) runCameraBenchmark(
	camera string,
	cam config.CameraConfig,
	frigateURL, go2rtcURL string,
	visionClient *vision.Client,
	globalPrompt string,
	withVision bool,
	promptOverride string,
	onResult func(BenchmarkResult),
) []BenchmarkResult {
	// Resolve the prompt to use: query param override > camera prompt > global prompt.
	resolvePrompt := func() string {
		if promptOverride != "" {
			return promptOverride
		}
		return resolveVisionPrompt("", cam.VisionPrompt != "", cam.VisionPrompt, globalPrompt)
	}

	// Warmup: when vision=true, do one untimed vision call first so the model
	// is loaded into VRAM. Without this, the first method pays the cold-start
	// penalty (model loading) and appears unfairly slow compared to the rest.
	if withVision {
		warmupData, wErr := grabFrameViaGo2rtcAPI(go2rtcURL, cam.VisionStream, 10*time.Second)
		if wErr != nil && frigateURL != "" {
			snapURL := fmt.Sprintf("%s/api/%s/latest.jpg?h=720", frigateURL, camera)
			resp, err := (&http.Client{Timeout: 30 * time.Second}).Get(snapURL)
			if err == nil {
				warmupData, _ = io.ReadAll(resp.Body)
				resp.Body.Close()
			}
		}
		if len(warmupData) > 0 {
			_, _ = visionClient.Describe(warmupData, "image/jpeg", resolvePrompt())
		}
	}

	var results []BenchmarkResult

	// Helper to time a snapshot method.
	tryMethod := func(name string, fn func() ([]byte, error)) {
		start := time.Now()
		data, err := fn()
		r := BenchmarkResult{
			Method:  name,
			OK:      err == nil,
			SnapSec: time.Since(start).Seconds(),
			Bytes:   len(data),
		}
		if err != nil {
			r.Error = err.Error()
			results = append(results, r)
			if onResult != nil {
				onResult(r)
			}
			return
		}

		if withVision {
			vStart := time.Now()
			desc, vErr := visionClient.Describe(data, "image/jpeg", resolvePrompt())
			r.VisionSec = time.Since(vStart).Seconds()
			r.TotalSec = r.SnapSec + r.VisionSec
			if vErr != nil {
				r.OK = false
				r.Error = fmt.Sprintf("vision failed: %s", vErr)
			} else {
				if len(desc) > 120 {
					desc = desc[:120] + "…"
				}
				r.Preview = desc
			}
		}
		// Decode JPEG dimensions AFTER timing so it doesn't affect measurements.
		r.Width, r.Height = decodeJPEGDimensions(data)
		// Include a base64 thumbnail for UI hover preview.
		r.Image = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data)
		results = append(results, r)
		if onResult != nil {
			onResult(r)
		}
	}

	// 1. Direct camera API (ISAPI for Hikvision, HTTP API for Reolink)
	if (cam.Type == "hikvision" || cam.Type == "reolink") && cam.IP != "" && cam.User != "" {
		switch cam.Type {
		case "hikvision":
			hikCam := cameras.NewHikvisionClient(cam.IP, cam.User, cam.Pass, cam.Channel, camera)
			tryMethod("isapi_main", func() ([]byte, error) {
				return hikCam.Snapshot("main")
			})
			tryMethod("isapi_sub", func() ([]byte, error) {
				return hikCam.Snapshot("sub")
			})
			// RTSP frame grab — some Hikvision cameras return the main stream
			// resolution from the ISAPI /picture endpoint regardless of channel.
			// RTSP delivers the actual configured resolution per channel.
			rtspMain := fmt.Sprintf(
				"rtsp://%s:%s@%s:554/Streaming/channels/%d01",
				cam.User, cam.Pass, cam.IP, cam.Channel,
			)
			rtspSub := fmt.Sprintf(
				"rtsp://%s:%s@%s:554/Streaming/channels/%d02",
				cam.User, cam.Pass, cam.IP, cam.Channel,
			)
			tryMethod("rtsp_main", func() ([]byte, error) {
				return grabRTSPFrame(rtspMain, 10*time.Second)
			})
			tryMethod("rtsp_sub", func() ([]byte, error) {
				return grabRTSPFrame(rtspSub, 10*time.Second)
			})
		case "reolink":
			reoCam := cameras.NewReolinkClient(cam.IP, cam.User, cam.Pass)
			tryMethod("reolink_main", func() ([]byte, error) {
				return reoCam.Snapshot("main")
			})
			tryMethod("reolink_sub", func() ([]byte, error) {
				return reoCam.Snapshot("sub")
			})
		}
	}

	// 2. go2rtc frame.jpeg (if vision_stream or go2rtc stream configured)
	streamName := cam.VisionStream
	if streamName == "" && cam.Stream != "" {
		streamName = cam.Stream
	}
	if streamName != "" && go2rtcURL != "" {
		tryMethod("go2rtc_frame", func() ([]byte, error) {
			return grabFrameViaGo2rtcAPI(go2rtcURL, streamName, 10*time.Second)
		})
	}

	// 3. go2rtc via ffmpeg (same stream, but ffmpeg path)
	if streamName != "" && go2rtcURL != "" {
		width := cam.VisionWidth
		if width <= 0 {
			width = 1280
		}
		tryMethod("go2rtc_ffmpeg", func() ([]byte, error) {
			return grabFrameViaFFmpeg(go2rtcURL, streamName, width, 10*time.Second)
		})
	}

	// 4. Frigate latest.jpg
	if frigateURL != "" {
		tryMethod("frigate", func() ([]byte, error) {
			snapURL := fmt.Sprintf("%s/api/%s/latest.jpg?h=720", frigateURL, camera)
			resp, err := (&http.Client{Timeout: 30 * time.Second}).Get(snapURL)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				return nil, fmt.Errorf("frigate returned HTTP %d", resp.StatusCode)
			}
			return io.ReadAll(resp.Body)
		})
	}

	return results
}

// SnapshotBenchmark handles GET /api/snapshot/:camera/benchmark — tries all
// available snapshot methods for the camera, times each one, and returns
// results sorted by latency. With ?vision=true, also runs the vision model
// against each captured frame to measure the full pipeline (snapshot + vision).
func (h *Handlers) SnapshotBenchmark(c echo.Context) error {
	log := h.logger(c)
	camera := c.Param("camera")
	if camera == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "camera required")
	}

	withVision := c.QueryParam("vision") == "true"
	promptOverride := c.QueryParam("prompt")

	h.cfgMu.Lock()
	cam := h.cfg.Cameras[camera]
	frigateURL := h.cfg.FrigateURL
	go2rtcURL := h.cfg.Go2rtcURL
	visionClient := h.vision
	globalPrompt := h.cfg.Vision.Prompt
	h.cfgMu.Unlock()

	if withVision && visionClient == nil {
		return echo.NewHTTPError(
			http.StatusServiceUnavailable,
			"vision model not configured (needed for ?vision=true)",
		)
	}

	results := h.runCameraBenchmark(
		camera, cam, frigateURL, go2rtcURL, visionClient, globalPrompt,
		withVision, promptOverride, nil,
	)

	if len(results) == 0 {
		return echo.NewHTTPError(http.StatusServiceUnavailable,
			"no snapshot methods available for this camera")
	}

	log.Info("snapshot: benchmark", "camera", camera, "methods", len(results), "vision", withVision)
	return c.JSON(http.StatusOK, map[string]any{
		"camera":  camera,
		"vision":  withVision,
		"results": results,
	})
}

// cachedCapture holds a single captured image with timing/metadata.
type cachedCapture struct {
	camera  string
	method  string
	data    []byte
	snapSec float64
	bytes   int
	width   int
	height  int
	err     error
}

// BenchmarkStream handles POST /api/benchmark — runs the full matrix of
// cameras × prompts × capture methods × models, streaming results as SSE.
//
// Two-phase approach to ensure fair vision timing:
//
//	Phase 1: Capture all images (per camera × method), cache them.
//	         Only capture time is measured — no model involvement.
//	Phase 2: For each model, warm it up (load into VRAM), then run through
//	         ALL cached images × prompts. The model stays loaded for all
//	         its tests, so no reload time leaks into vision_sec.
//
// This is critical because Lemonade (llama.cpp) can only hold one model in
// VRAM at a time. If we switched models per-image, each switch would evict
// the previous model and the reload time would be counted as "vision time",
// making the comparison unfair.
//
// SSE events:
//
//	start    — { total_steps, cameras, prompts, models, with_vision }
//	log      — { msg } — human-readable progress message for the live log feed
//	progress — { step, total, camera, prompt, method, model }
//	result   — { camera, prompt, model, results: [BenchmarkResult] }
//	done     — { total_steps, elapsed_sec }
func (h *Handlers) BenchmarkStream(c echo.Context) error {
	log := h.logger(c)

	var req struct {
		Cameras    []string `json:"cameras"` // empty = all enabled
		Prompts    []string `json:"prompts"`
		Models     []string `json:"models"` // empty = configured model only
		WithVision bool     `json:"with_vision"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}

	h.cfgMu.Lock()
	allCameras := make(map[string]config.CameraConfig, len(h.cfg.Cameras))
	for name, cam := range h.cfg.Cameras {
		if cam.Enabled {
			allCameras[name] = cam
		}
	}
	frigateURL := h.cfg.FrigateURL
	go2rtcURL := h.cfg.Go2rtcURL
	visionClient := h.vision
	globalPrompt := h.cfg.Vision.Prompt
	configuredModel := h.cfg.Vision.Model
	h.cfgMu.Unlock()

	if req.WithVision && visionClient == nil {
		return echo.NewHTTPError(
			http.StatusServiceUnavailable,
			"vision model not configured (needed for with_vision=true)",
		)
	}

	// Resolve which cameras to test
	camerasToTest := make(map[string]config.CameraConfig)
	if len(req.Cameras) > 0 {
		for _, name := range req.Cameras {
			if cam, ok := allCameras[name]; ok {
				camerasToTest[name] = cam
			}
		}
	} else {
		camerasToTest = allCameras
	}

	// Resolve which models to test
	modelsToTest := req.Models
	if len(modelsToTest) == 0 && configuredModel != "" {
		modelsToTest = []string{configuredModel}
	}

	// SSE streaming
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().WriteHeader(http.StatusOK)

	flusher, ok := c.Response().Writer.(http.Flusher)

	writeEvent := func(eventType string, data any) {
		jsonData, _ := json.Marshal(data)
		fmt.Fprintf(c.Response().Writer, "event: %s\ndata: %s\n\n", eventType, jsonData)
		if ok {
			flusher.Flush()
		}
	}

	writeLog := func(msg string) {
		log.Info("matrix: " + msg)
		writeEvent("log", map[string]string{"msg": msg})
	}

	// --- Phase 1: Capture all images (no model involvement) ---
	var captures []cachedCapture
	writeLog("Phase 1: Capturing all images (no model involvement)")

	for camName, cam := range camerasToTest {
		caps := h.captureAllMethods(camName, cam, frigateURL, go2rtcURL)
		for _, cap := range caps {
			captures = append(captures, cap)
			if cap.err != nil {
				writeLog(fmt.Sprintf("  Capture FAILED %s / %s: %s", camName, cap.method, cap.err))
			} else {
				writeLog(fmt.Sprintf("  Captured %s / %s (%.2fs, %d bytes)",
					camName, cap.method, cap.snapSec, cap.bytes))
			}
		}
	}
	writeLog(fmt.Sprintf("Phase 1 complete: %d images captured", len(captures)))

	// --- Phase 2: For each model, warm up then run through all cached images ---
	totalSteps := len(captures) * len(req.Prompts) * len(modelsToTest)
	writeEvent("start", map[string]any{
		"total_steps": totalSteps,
		"cameras":     len(camerasToTest),
		"prompts":     len(req.Prompts),
		"models":      modelsToTest,
		"with_vision": req.WithVision,
		"captures":    len(captures),
	})

	startTime := time.Now()
	step := 0

	if !req.WithVision {
		// No vision: just report capture results (one per capture × prompt)
		for _, cap := range captures {
			for _, prompt := range req.Prompts {
				step++
				r := BenchmarkResult{
					Method:  cap.method,
					OK:      cap.err == nil,
					SnapSec: cap.snapSec,
					Bytes:   cap.bytes,
					Width:   cap.width,
					Height:  cap.height,
				}
				if cap.err != nil {
					r.Error = cap.err.Error()
				}
				writeEvent("progress", map[string]any{
					"step": step, "total": totalSteps,
					"camera": cap.camera, "prompt": prompt,
					"method": cap.method, "model": "",
				})
				writeEvent("result", map[string]any{
					"camera": cap.camera, "prompt": prompt, "model": "",
					"results": []BenchmarkResult{r},
				})
			}
		}
	} else {
		// Model is the OUTER loop so it stays loaded in VRAM for all its tests.
		for _, model := range modelsToTest {
			writeLog(fmt.Sprintf("Phase 2: Loading model %s into VRAM", model))

			// Warm up this model with the first good capture
			if len(captures) > 0 {
				for _, cap := range captures {
					if cap.err == nil && len(cap.data) > 0 {
						warmupStart := time.Now()
						_, _ = visionClient.DescribeWithModel(
							cap.data, "image/jpeg", "warmup", model,
						)
						writeLog(fmt.Sprintf("  Model %s loaded in %.2fs",
							model, time.Since(warmupStart).Seconds()))
						break
					}
				}
			}

			writeLog(fmt.Sprintf("  Running %d images × %d prompts through model %s",
				len(captures), len(req.Prompts), model))

			for _, cap := range captures {
				for _, prompt := range req.Prompts {
					step++

					if cap.err != nil {
						r := BenchmarkResult{
							Method:  cap.method,
							OK:      false,
							SnapSec: cap.snapSec,
							Model:   model,
							Error:   cap.err.Error(),
						}
						writeEvent("progress", map[string]any{
							"step": step, "total": totalSteps,
							"camera": cap.camera, "prompt": prompt,
							"method": cap.method, "model": model,
						})
						writeEvent("result", map[string]any{
							"camera": cap.camera, "prompt": prompt, "model": model,
							"results": []BenchmarkResult{r},
						})
						continue
					}

					resolvePrompt := func() string {
						if prompt != "" {
							return prompt
						}
						cam := camerasToTest[cap.camera]
						return resolveVisionPrompt("", cam.VisionPrompt != "", cam.VisionPrompt, globalPrompt)
					}

					vStart := time.Now()
					desc, vErr := visionClient.DescribeWithModel(
						cap.data, "image/jpeg", resolvePrompt(), model,
					)
					visionSec := time.Since(vStart).Seconds()

					r := BenchmarkResult{
						Method:    cap.method,
						OK:        vErr == nil,
						SnapSec:   cap.snapSec,
						VisionSec: visionSec,
						TotalSec:  cap.snapSec + visionSec,
						Bytes:     cap.bytes,
						Width:     cap.width,
						Height:    cap.height,
						Model:     model,
						Image:     "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(cap.data),
					}
					if vErr != nil {
						r.Error = fmt.Sprintf("vision failed: %s", vErr)
					} else {
						if len(desc) > 120 {
							desc = desc[:120] + "…"
						}
						r.Preview = desc
					}

					writeEvent("progress", map[string]any{
						"step": step, "total": totalSteps,
						"camera": cap.camera, "prompt": prompt,
						"method": cap.method, "model": model,
					})
					writeEvent("result", map[string]any{
						"camera": cap.camera, "prompt": prompt, "model": model,
						"results": []BenchmarkResult{r},
					})
				}
			}
			writeLog(fmt.Sprintf("  Model %s complete", model))
		}
	}

	elapsed := time.Since(startTime).Seconds()
	writeEvent("done", map[string]any{
		"total_steps": step,
		"elapsed_sec": elapsed,
	})
	writeLog(fmt.Sprintf("Matrix complete: %d results in %.1fs", step, elapsed))

	return nil
}

// captureAllMethods runs all available capture methods for a camera and returns
// the captured images with timing/metadata. No vision is involved.
func (h *Handlers) captureAllMethods(
	camera string,
	cam config.CameraConfig,
	frigateURL, go2rtcURL string,
) []cachedCapture {
	var captures []cachedCapture

	doCapture := func(name string, fn func() ([]byte, error)) {
		start := time.Now()
		data, err := fn()
		c := cachedCapture{
			camera:  camera,
			method:  name,
			data:    data,
			snapSec: time.Since(start).Seconds(),
			bytes:   len(data),
			err:     err,
		}
		if err == nil && len(data) > 0 {
			c.width, c.height = decodeJPEGDimensions(data)
		}
		captures = append(captures, c)
	}

	// 1. Direct camera API (ISAPI for Hikvision, HTTP API for Reolink)
	if (cam.Type == "hikvision" || cam.Type == "reolink") && cam.IP != "" && cam.User != "" {
		switch cam.Type {
		case "hikvision":
			hikCam := cameras.NewHikvisionClient(cam.IP, cam.User, cam.Pass, cam.Channel, camera)
			doCapture("isapi_main", func() ([]byte, error) { return hikCam.Snapshot("main") })
			doCapture("isapi_sub", func() ([]byte, error) { return hikCam.Snapshot("sub") })
			rtspMain := fmt.Sprintf(
				"rtsp://%s:%s@%s:554/Streaming/channels/%d01",
				cam.User, cam.Pass, cam.IP, cam.Channel,
			)
			rtspSub := fmt.Sprintf(
				"rtsp://%s:%s@%s:554/Streaming/channels/%d02",
				cam.User, cam.Pass, cam.IP, cam.Channel,
			)
			doCapture("rtsp_main", func() ([]byte, error) { return grabRTSPFrame(rtspMain, 10*time.Second) })
			doCapture("rtsp_sub", func() ([]byte, error) { return grabRTSPFrame(rtspSub, 10*time.Second) })
		case "reolink":
			reoCam := cameras.NewReolinkClient(cam.IP, cam.User, cam.Pass)
			doCapture("reolink_main", func() ([]byte, error) { return reoCam.Snapshot("main") })
			doCapture("reolink_sub", func() ([]byte, error) { return reoCam.Snapshot("sub") })
		}
	}

	// 2. go2rtc frame.jpeg
	streamName := cam.VisionStream
	if streamName == "" && cam.Stream != "" {
		streamName = cam.Stream
	}
	if streamName != "" && go2rtcURL != "" {
		doCapture("go2rtc_frame", func() ([]byte, error) {
			return grabFrameViaGo2rtcAPI(go2rtcURL, streamName, 10*time.Second)
		})
	}

	// 3. go2rtc via ffmpeg
	if streamName != "" && go2rtcURL != "" {
		width := cam.VisionWidth
		if width <= 0 {
			width = 1280
		}
		doCapture("go2rtc_ffmpeg", func() ([]byte, error) {
			return grabFrameViaFFmpeg(go2rtcURL, streamName, width, 10*time.Second)
		})
	}

	// 4. Frigate
	if frigateURL != "" {
		doCapture("frigate", func() ([]byte, error) {
			snapURL := fmt.Sprintf("%s/api/%s/latest.jpg?h=720", frigateURL, camera)
			resp, err := (&http.Client{Timeout: 30 * time.Second}).Get(snapURL)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				return nil, fmt.Errorf("frigate returned HTTP %d", resp.StatusCode)
			}
			return io.ReadAll(resp.Body)
		})
	}

	return captures
}

// fetchSnapshot grabs a JPEG frame for a camera, using the camera's configured
// vision_stream if set (via ffmpeg from go2rtc), otherwise falls back to
// Frigate's latest.jpg (detect stream). For Hikvision cameras, it tries the
// direct ISAPI snapshot endpoint first (fastest, no intermediate service).
// For Reolink cameras, it tries the direct Reolink HTTP API snapshot.
// streamOverride, if non-empty, selects "main" or "sub" for ISAPI/Reolink
// snapshots or overrides the go2rtc stream name.
// cam.SnapMethod, if set to "isapi", "go2rtc", or "frigate", forces that method.
func (h *Handlers) fetchSnapshot(
	ctx context.Context,
	cameraName string,
	cam config.CameraConfig,
	frigateURL string,
	streamOverride string,
) ([]byte, error) {
	method := cam.SnapMethod
	if method == "" {
		method = "auto"
	}

	// Helper: try direct camera API (ISAPI for Hikvision, HTTP API for Reolink).
	tryCameraAPI := func() ([]byte, error) {
		streamType := "sub"
		if streamOverride == "main" {
			streamType = "main"
		}
		switch cam.Type {
		case "hikvision":
			if cam.IP == "" || cam.User == "" {
				return nil, fmt.Errorf("hikvision credentials not configured")
			}
			hikCam := cameras.NewHikvisionClient(
				cam.IP, cam.User, cam.Pass, cam.Channel, cameraName,
			)
			return hikCam.Snapshot(streamType)
		case "reolink":
			if cam.IP == "" || cam.User == "" {
				return nil, fmt.Errorf("reolink credentials not configured")
			}
			reoCam := cameras.NewReolinkClient(cam.IP, cam.User, cam.Pass)
			return reoCam.Snapshot(streamType)
		default:
			return nil, fmt.Errorf("no direct snapshot API for camera type %s", cam.Type)
		}
	}

	// Helper: try go2rtc.
	tryGo2rtc := func() ([]byte, error) {
		streamName := cam.VisionStream
		if streamOverride != "" && streamOverride != "main" && streamOverride != "sub" {
			streamName = streamOverride
		}
		if streamName == "" {
			streamName = cam.Stream
		}
		if streamName == "" || h.cfg.Go2rtcURL == "" {
			return nil, fmt.Errorf("no go2rtc stream configured for camera %s", cameraName)
		}
		width := cam.VisionWidth
		if width <= 0 {
			width = 1280
		}
		return grabFrameFromStream(h.cfg.Go2rtcURL, streamName, width, 10*time.Second)
	}

	// Helper: try Frigate.
	tryFrigate := func() ([]byte, error) {
		if frigateURL == "" {
			return nil, fmt.Errorf("frigate URL not configured")
		}
		snapURL := fmt.Sprintf("%s/api/%s/latest.jpg?h=720", frigateURL, cameraName)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, snapURL, nil)
		if err != nil {
			return nil, fmt.Errorf("building frigate request: %w", err)
		}
		resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
		if err != nil {
			return nil, fmt.Errorf("frigate snapshot: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("frigate returned HTTP %d", resp.StatusCode)
		}
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("reading snapshot: %w", err)
		}
		return data, nil
	}

	// Order depends on snap_method.
	switch method {
	case "isapi":
		if data, err := tryCameraAPI(); err == nil {
			return data, nil
		}
		if data, err := tryGo2rtc(); err == nil {
			return data, nil
		}
		return tryFrigate()

	case "go2rtc":
		if data, err := tryGo2rtc(); err == nil {
			return data, nil
		}
		if data, err := tryCameraAPI(); err == nil {
			return data, nil
		}
		return tryFrigate()

	case "frigate":
		return tryFrigate()

	default: // "auto"
		// For camera types with direct API support, try that first (fastest).
		if cam.Type == "hikvision" || cam.Type == "reolink" {
			if data, err := tryCameraAPI(); err == nil {
				return data, nil
			}
		}
		if data, err := tryGo2rtc(); err == nil {
			return data, nil
		}
		return tryFrigate()
	}
}

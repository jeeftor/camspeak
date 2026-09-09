package api

import (
	"context"
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

// SnapshotBenchmark handles GET /api/snapshot/:camera/benchmark — tries all
// available snapshot methods for the camera, times each one, and returns
// results sorted by latency. Useful for picking the fastest method per camera.
func (h *Handlers) SnapshotBenchmark(c echo.Context) error {
	log := h.logger(c)
	camera := c.Param("camera")
	if camera == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "camera required")
	}

	h.cfgMu.Lock()
	cam := h.cfg.Cameras[camera]
	frigateURL := h.cfg.FrigateURL
	go2rtcURL := h.cfg.Go2rtcURL
	h.cfgMu.Unlock()

	type result struct {
		Method string `json:"method"`
		OK     bool   `json:"ok"`
		Ms     int64  `json:"ms"`
		Bytes  int    `json:"bytes"`
		Error  string `json:"error,omitempty"`
	}

	var results []result

	// Helper to time a method.
	tryMethod := func(name string, fn func() ([]byte, error)) {
		start := time.Now()
		data, err := fn()
		r := result{
			Method: name,
			OK:     err == nil,
			Ms:     time.Since(start).Milliseconds(),
			Bytes:  len(data),
		}
		if err != nil {
			r.Error = err.Error()
		}
		results = append(results, r)
	}

	// 1. ISAPI main stream (Hikvision only)
	if cam.Type == "hikvision" && cam.IP != "" && cam.User != "" {
		hikCam := cameras.NewHikvisionClient(cam.IP, cam.User, cam.Pass, cam.Channel, camera)
		tryMethod("isapi_main", func() ([]byte, error) {
			return hikCam.Snapshot("main")
		})
	}

	// 2. ISAPI sub stream (Hikvision only)
	if cam.Type == "hikvision" && cam.IP != "" && cam.User != "" {
		hikCam := cameras.NewHikvisionClient(cam.IP, cam.User, cam.Pass, cam.Channel, camera)
		tryMethod("isapi_sub", func() ([]byte, error) {
			return hikCam.Snapshot("sub")
		})
	}

	// 3. go2rtc frame.jpeg (if vision_stream or go2rtc stream configured)
	streamName := cam.VisionStream
	if streamName == "" && cam.Stream != "" {
		streamName = cam.Stream
	}
	if streamName != "" && go2rtcURL != "" {
		tryMethod("go2rtc_frame", func() ([]byte, error) {
			return grabFrameViaGo2rtcAPI(go2rtcURL, streamName, 10*time.Second)
		})
	}

	// 4. go2rtc via ffmpeg (same stream, but ffmpeg path)
	if streamName != "" && go2rtcURL != "" {
		width := cam.VisionWidth
		if width <= 0 {
			width = 1280
		}
		tryMethod("go2rtc_ffmpeg", func() ([]byte, error) {
			return grabFrameViaFFmpeg(go2rtcURL, streamName, width, 10*time.Second)
		})
	}

	// 5. Frigate latest.jpg
	if frigateURL != "" {
		tryMethod("frigate", func() ([]byte, error) {
			snapURL := fmt.Sprintf("%s/api/%s/latest.jpg?h=720", frigateURL, camera)
			ctx, cancel := context.WithTimeout(c.Request().Context(), 30*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, snapURL, nil)
			if err != nil {
				return nil, err
			}
			resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
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

	if len(results) == 0 {
		return echo.NewHTTPError(http.StatusServiceUnavailable,
			"no snapshot methods available for this camera")
	}

	log.Info("snapshot: benchmark", "camera", camera, "methods", len(results))
	return c.JSON(http.StatusOK, map[string]any{
		"camera":  camera,
		"results": results,
	})
}

// fetchSnapshot grabs a JPEG frame for a camera, using the camera's configured
// vision_stream if set (via ffmpeg from go2rtc), otherwise falls back to
// Frigate's latest.jpg (detect stream). For Hikvision cameras, it tries the
// direct ISAPI snapshot endpoint first (fastest, no intermediate service).
// streamOverride, if non-empty, selects "main" or "sub" for ISAPI snapshots
// or overrides the go2rtc stream name.
func (h *Handlers) fetchSnapshot(
	ctx context.Context,
	cameraName string,
	cam config.CameraConfig,
	frigateURL string,
	streamOverride string,
) ([]byte, error) {
	// For Hikvision cameras, try the direct ISAPI snapshot first.
	// This bypasses go2rtc/Frigate entirely and is typically the fastest path.
	if cam.Type == "hikvision" && cam.IP != "" && cam.User != "" {
		streamType := "sub"
		if streamOverride == "main" {
			streamType = "main"
		}
		hikCam := cameras.NewHikvisionClient(
			cam.IP, cam.User, cam.Pass, cam.Channel, cameraName,
		)
		if data, err := hikCam.Snapshot(streamType); err == nil {
			return data, nil
		}
		// Fall through to go2rtc/Frigate on failure
	}

	// If the camera has a vision_stream configured, use ffmpeg to grab from go2rtc.
	streamName := cam.VisionStream
	if streamOverride != "" && streamOverride != "main" && streamOverride != "sub" {
		streamName = streamOverride
	}
	if streamName != "" && h.cfg.Go2rtcURL != "" {
		width := cam.VisionWidth
		if width <= 0 {
			width = 1280 // sensible default for vision models
		}
		return grabFrameFromStream(h.cfg.Go2rtcURL, streamName, width, 10*time.Second)
	}

	// Fall back to Frigate detect stream.
	if frigateURL == "" {
		return nil, fmt.Errorf(
			"frigate URL not configured and no vision_stream set for camera %s",
			cameraName,
		)
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

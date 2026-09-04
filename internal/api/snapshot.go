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

// fetchSnapshot grabs a JPEG frame for a camera, using the camera's configured
// vision_stream if set (via ffmpeg from go2rtc), otherwise falls back to
// Frigate's latest.jpg (detect stream).
func (h *Handlers) fetchSnapshot(
	ctx context.Context,
	cameraName string,
	cam config.CameraConfig,
	frigateURL string,
) ([]byte, error) {
	// If the camera has a vision_stream configured, use ffmpeg to grab from go2rtc.
	if cam.VisionStream != "" && h.cfg.Go2rtcURL != "" {
		width := cam.VisionWidth
		if width <= 0 {
			width = 1280 // sensible default for vision models
		}
		return grabFrameFromStream(h.cfg.Go2rtcURL, cam.VisionStream, width, 10*time.Second)
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

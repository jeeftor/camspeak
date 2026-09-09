package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	clog "github.com/charmbracelet/log"
	"github.com/labstack/echo/v4"

	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/library"
)

// Broadcast handles POST /api/broadcast — TTS or preset → all cameras in parallel.
func (h *Handlers) Broadcast(c echo.Context) error {
	log := h.logger(c)

	var req broadcastReq
	err := c.Bind(&req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid body")
	}

	if req.Text == "" && req.Preset == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "text or preset required")
	}

	names := h.reg.Names()
	mode := "tts"
	if req.Preset != "" {
		mode = "preset"
	}
	log.Info("broadcast: starting", "mode", mode, "cameras", names, "text_len", len(req.Text))
	start := time.Now()

	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)

	errs := make([]string, 0)
	succeeded := make([]string, 0)

	for _, name := range names {
		wg.Add(1)
		go func(cam string) {
			defer wg.Done()

			camStart := time.Now()
			var err error
			if req.Preset != "" {
				_, err = h.playPreset(log, cam, req.Category, req.Preset, req.Gain, req.Loop)
			} else {
				_, err = h.speakText(log, cam, req.Text, req.Voice, req.Gain)
			}

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				log.Error(
					"broadcast: camera failed",
					"camera",
					cam,
					"elapsed",
					time.Since(camStart),
					"err",
					err,
				)
				errs = append(errs, fmt.Sprintf("%s: %s", cam, err))
			} else {
				log.Info("broadcast: camera done", "camera", cam, "elapsed", time.Since(camStart))
				succeeded = append(succeeded, cam)
			}
		}(name)
	}

	wg.Wait()

	log.Info(
		"broadcast: complete",
		"succeeded",
		len(succeeded),
		"failed",
		len(errs),
		"elapsed",
		time.Since(start),
	)

	if len(errs) > 0 {
		return c.JSON(http.StatusMultiStatus, map[string]any{
			"succeeded": succeeded,
			"errors":    errs,
		})
	}

	return c.JSON(http.StatusOK, map[string]any{
		"status":    "ok",
		"succeeded": succeeded,
	})
}

// --- Internal helpers ---

// resolveGainValue returns the numeric gain for a camera call. If reqGain > 0
// it wins; otherwise the camera's current runtime gain is used; otherwise the
// default (3.0) is returned. Shared by gainForCall and effectiveGain.
func (h *Handlers) resolveGainValue(camera string, reqGain float64) float64 {
	if reqGain > 0 {
		return reqGain
	}
	if gc := h.reg.GetGain(camera); gc != nil {
		return gc.Get()
	}
	return 3.0
}

// gainForCall returns a GainController for a one-off audio send. If reqGain > 0
// that value is used; otherwise the camera's current runtime gain controller is
// returned (so runtime volume changes apply per-chunk during playback).
func (h *Handlers) gainForCall(camera string, reqGain float64) *cameras.GainController {
	if reqGain > 0 {
		return cameras.NewGainController(reqGain)
	}
	if gc := h.reg.GetGain(camera); gc != nil {
		return gc
	}
	return cameras.NewGainController(3.0)
}

// sendRawWithLevel wraps cam.SendRaw with a VU meter level sink so that
// one-shot playback (speak, play, play-url, beep, describe, announce)
// feeds real-time audio levels into the /api/stream-levels SSE endpoint.
// The sink is attached to the GainController before SendRaw and cleared
// after, along with the one-shot level entry.
func sendRawWithLevel(
	camera string,
	cam cameras.Speaker,
	rawFile string,
	gc *cameras.GainController,
) (cameras.SendTiming, error) {
	if gc != nil {
		gc.SetLevelSink(func(level float64) {
			setOneShotLevel(camera, level)
		})
		defer gc.SetLevelSink(nil)
		defer clearOneShotLevel(camera)
	}
	return cam.SendRaw(rawFile, gc)
}

// effectiveGain returns the numeric gain to use for ffmpeg-based paths (streams
// and looped presets). If reqGain > 0 it wins; otherwise the camera's current
// runtime gain is used.
func (h *Handlers) effectiveGain(camera string, reqGain float64) float64 {
	return h.resolveGainValue(camera, reqGain)
}

func (h *Handlers) speakText(
	log *clog.Logger,
	cameraName, text, voice string,
	gain float64,
) (*StepTimings, error) {
	t := NewStepTimings(3)
	cam, err := h.reg.Get(cameraName)
	if err != nil {
		return t, err
	}

	if voice == "" {
		voice = h.cfg.TTS.DefaultVoice
	}

	// Gain is now applied at send time via GainController (per-chunk).
	// Transcode at unity so the raw file is clean; volume is adjusted live.

	ttsStart := time.Now()
	wav, err := h.tts.Speak(text, voice)
	if err != nil {
		return t, fmt.Errorf("TTS: %w", err)
	}
	t.Add("tts_ms", ttsStart)
	log.Debug(
		"speak: TTS generated",
		"camera",
		cameraName,
		"voice",
		voice,
		"wav_bytes",
		len(wav),
		"elapsed",
		time.Since(ttsStart),
	)

	transcodeStart := time.Now()
	rawPath, err := wavBytesToRawWithPrime(wav, h.tmpDir, 1.0, h.cfg.PrimeSilenceMs)
	if err != nil {
		return t, fmt.Errorf("transcoding: %w", err)
	}
	t.Add("transcode_ms", transcodeStart)
	defer os.Remove(rawPath)

	log.Debug("speak: sending to camera", "camera", cameraName)
	setPlayback(cameraName, "speak", text)
	sendTiming, err := sendRawWithLevel(cameraName, cam, rawPath, h.gainForCall(cameraName, gain))
	if err != nil {
		clearPlayback(cameraName)
		return t, fmt.Errorf("sending to camera: %w", err)
	}
	t.steps["send_open_ms"] = time.Duration(sendTiming.OpenMs) * time.Millisecond
	t.steps["send_playback_ms"] = time.Duration(sendTiming.PlaybackMs) * time.Millisecond
	log.Debug(
		"speak: camera send complete",
		"camera",
		cameraName,
		"open_ms",
		sendTiming.OpenMs,
		"playback_ms",
		sendTiming.PlaybackMs,
	)

	h.events.publish(
		event{Camera: cameraName, Action: "speak", Text: text, Voice: voice, At: time.Now()},
	)
	clearPlayback(cameraName)

	return t, nil
}

func (h *Handlers) playPreset(
	log *clog.Logger,
	cameraName, category, presetName string,
	gain float64,
	loop int,
) (*StepTimings, error) {
	t := NewStepTimings(3)
	cam, err := h.reg.Get(cameraName)
	if err != nil {
		return t, err
	}

	loadStart := time.Now()
	var preset *library.Preset
	if category != "" {
		preset, err = h.store.Get(category, presetName)
	} else {
		preset, err = h.store.GetByName(presetName)
	}

	if err != nil {
		return t, err
	}
	t.Add("load_ms", loadStart)

	// Stream preset: resolve playlist URL and start ffmpeg → camera stream.
	if preset.IsStream() {
		if loop != 0 {
			log.Debug(
				"play: loop ignored for stream preset",
				"camera",
				cameraName,
				"preset",
				preset.Name,
				"loop",
				loop,
			)
		}
		streamURL, err := resolveStreamURL(preset.URL)
		if err != nil {
			log.Warn("play: failed to resolve stream preset playlist", "url", preset.URL, "err", err)
			return t, err
		}
		err = h.startStreamToCamera(log, cam, cameraName, streamURL, preset.URL, gain)
		if err != nil {
			return t, err
		}
		log.Info("play: stream preset started", "camera", cameraName, "preset", preset.Name, "url", preset.URL)
		return t, nil
	}

	// Audio preset: send raw file to camera (existing path).
	// Gain is now applied at send time (per-chunk in SendRaw via GainController),
	// so we no longer need to pre-transcode with boostRawGain. The raw preset
	// file is sent as-is; the GainController scales each 100ms chunk in real-time.
	sendPath := preset.RawPath

	// Apply per-preset gain on top of camera/request gain.
	if preset.Gain > 0 && preset.Gain != 1.0 {
		baseGain := h.effectiveGain(cameraName, gain)
		gain = baseGain * preset.Gain
	}

	if loop != 0 {
		// Looped presets use ffmpeg's adelay filter for prime silence, so we
		// pass the original raw file. (Creating a temp file with prepended
		// silence here would race with deletion: playPresetLooped starts
		// ffmpeg asynchronously and returns, so a defer os.Remove would
		// delete the temp file before ffmpeg could read it.)
		return h.playPresetLooped(log, cam, cameraName, preset, sendPath, t, gain, loop)
	}

	// Prepend prime silence to warm the camera's audio engine.
	// (Only for the non-looped SendRaw path; looped presets use adelay.)
	if h.cfg.PrimeSilenceMs > 0 {
		primed, err := prependSilenceToNewFile(sendPath, h.tmpDir, h.cfg.PrimeSilenceMs)
		if err != nil {
			log.Warn("play: prime silence failed, sending without", "err", err)
		} else if primed != sendPath {
			defer os.Remove(primed)
			sendPath = primed
		}
	}

	log.Debug(
		"play: sending preset",
		"camera",
		cameraName,
		"preset",
		preset.Name,
		"raw_bytes",
		preset.Size,
		"gain",
		gain,
		"loop",
		loop,
	)

	setPlayback(cameraName, "play", preset.Name)
	sendTiming, err := sendRawWithLevel(cameraName, cam, sendPath, h.gainForCall(cameraName, gain))
	if err != nil {
		clearPlayback(cameraName)
		return t, fmt.Errorf("sending to camera: %w", err)
	}
	t.steps["send_open_ms"] = time.Duration(sendTiming.OpenMs) * time.Millisecond
	t.steps["send_playback_ms"] = time.Duration(sendTiming.PlaybackMs) * time.Millisecond
	log.Debug(
		"play: camera send complete",
		"camera",
		cameraName,
		"open_ms",
		sendTiming.OpenMs,
		"playback_ms",
		sendTiming.PlaybackMs,
	)

	h.events.publish(event{Camera: cameraName, Action: "play", Text: preset.Name, At: time.Now()})
	clearPlayback(cameraName)

	return t, nil
}

// playPresetLooped plays a preset in a loop using ffmpeg -stream_loop,
// piped to cam.Stream(). loop=-1 means infinite; loop=N (N>0) plays the
// preset N+1 times (once + N loops). This registers a streamSession
// so the loop can be paused, resumed, and stopped just like a live stream.
func (h *Handlers) playPresetLooped(
	log *clog.Logger,
	cam cameras.Speaker,
	cameraName string,
	preset *library.Preset,
	rawPath string,
	t *StepTimings,
	gain float64,
	loop int,
) (*StepTimings, error) {
	// Stop any existing ffmpeg stream for this camera first.
	stopStream(cameraName)

	ctx, cancel := context.WithCancel(context.Background())

	// Build audio filter: volume applies the requested/camera gain, and
	// adelay adds prime silence at the start of each loop iteration.
	g := h.effectiveGain(cameraName, gain)
	af := fmt.Sprintf("volume=%.2f", g)
	if h.cfg.PrimeSilenceMs > 0 {
		af = fmt.Sprintf("adelay=%d|%d,volume=%.2f", h.cfg.PrimeSilenceMs, h.cfg.PrimeSilenceMs, g)
	}

	args := []string{
		"-nostdin", "-loglevel", "error",
		"-stream_loop", strconv.Itoa(loop), // -1 = infinite, N = N+1 plays
		"-f", "mulaw", "-ar", "8000", "-ac", "1",
		"-i", rawPath,
	}
	if af != "" {
		args = append(args, "-af", af)
	}
	args = append(args,
		"-acodec", "pcm_mulaw",
		"-ar", "8000",
		"-ac", "1",
		"-f", "mulaw",
		"-",
	)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return t, fmt.Errorf("ffmpeg stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return t, fmt.Errorf("starting ffmpeg: %w", err)
	}

	detail := preset.Name + " (loop)"
	if loop > 0 {
		detail = fmt.Sprintf("%s (loop %dx)", preset.Name, loop+1)
	}
	session := &streamSession{
		cmd:     cmd,
		cancel:  cancel,
		url:     rawPath,
		started: now(),
	}
	activeStreamsMu.Lock()
	activeStreams[cameraName] = session
	activeStreamsMu.Unlock()
	setPlayback(cameraName, "play", detail)

	log.Info("play: looped preset started", "camera", cameraName, "preset", preset.Name)
	h.events.publish(event{Camera: cameraName, Action: "play", Text: detail, At: time.Now()})

	// Wrap stdout with a level tap so the VU meter can sample audio levels.
	tap := &levelTapReader{r: stdout, session: session}

	go func() {
		// Interrupt any active AirPlay/session right before acquiring the
		// camera mutex, minimizing the reconnect race window.
		_ = cam.Stop()
		streamErr := cam.Stream(tap)
		stopStream(cameraName)
		clearPlayback(cameraName)
		elapsed := time.Since(now())
		if streamErr != nil {
			log.Warn("play: looped preset ended with error",
				"camera", cameraName, "preset", preset.Name,
				"elapsed", elapsed, "err", streamErr)
		} else {
			log.Info("play: looped preset ended",
				"camera", cameraName, "preset", preset.Name, "elapsed", elapsed)
		}
	}()
	go func() {
		if err := cmd.Wait(); err != nil {
			log.Warn("play: looped preset ffmpeg exited with error",
				"camera", cameraName, "preset", preset.Name, "err", err)
		}
		stopStream(cameraName)
	}()

	return t, nil
}

// BroadcastToCameras sends text or preset to multiple cameras in parallel.
// Used by the broadcast endpoint and MCP broadcast tool.
func (h *Handlers) BroadcastToCameras(cams []string, text, preset, voice string, loop int) {
	var wg sync.WaitGroup
	for _, cam := range cams {
		wg.Add(1)
		go func(c string) {
			defer wg.Done()

			if preset != "" {
				_, _ = h.playPreset(h.log, c, "", preset, 0, loop)
			} else if text != "" {
				_, _ = h.speakText(h.log, c, text, voice, 0)
			}
		}(cam)
	}

	wg.Wait()
}

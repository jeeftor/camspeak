package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
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

func (h *Handlers) speakText(log *clog.Logger, cameraName, text, voice string, gain float64) (*StepTimings, error) {
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
	sendTiming, err := cam.SendRaw(rawPath, h.reg.GetGain(cameraName))
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

	h.events.publish(event{Camera: cameraName, Action: "speak", Text: text, Voice: voice, At: time.Now()})
	clearPlayback(cameraName)

	return t, nil
}

func (h *Handlers) playPreset(
	log *clog.Logger,
	cameraName, category, presetName string,
	gain float64,
	loop bool,
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

	// Gain is now applied at send time (per-chunk in SendRaw via GainController),
	// so we no longer need to pre-transcode with boostRawGain. The raw preset
	// file is sent as-is; the GainController scales each 100ms chunk in real-time.
	sendPath := preset.RawPath

	// Prepend prime silence to warm the camera's audio engine.
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

	if loop {
		return h.playPresetLooped(log, cam, cameraName, preset, sendPath, t)
	}

	setPlayback(cameraName, "play", preset.Name)
	sendTiming, err := cam.SendRaw(sendPath, h.reg.GetGain(cameraName))
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

// playPresetLooped plays a preset in an infinite loop using ffmpeg
// -stream_loop -1, piped to cam.Stream(). This registers a streamSession
// so the loop can be paused, resumed, and stopped just like a live stream.
func (h *Handlers) playPresetLooped(
	log *clog.Logger,
	cam cameras.Speaker,
	cameraName string,
	preset *library.Preset,
	rawPath string,
	t *StepTimings,
) (*StepTimings, error) {
	// Stop any existing stream for this camera first.
	stopStream(cameraName)

	ctx, cancel := context.WithCancel(context.Background())

	// Build audio filter: gain is already applied to rawPath if needed.
	// adelay adds prime silence at the start of each loop iteration.
	af := ""
	if h.cfg.PrimeSilenceMs > 0 {
		af = fmt.Sprintf("adelay=%d|%d", h.cfg.PrimeSilenceMs, h.cfg.PrimeSilenceMs)
	}

	args := []string{
		"-nostdin", "-loglevel", "error",
		"-stream_loop", "-1", // loop forever
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
	activeStreamsMu.Lock()
	activeStreams[cameraName] = &streamSession{cmd: cmd, cancel: cancel, url: rawPath, started: now()}
	activeStreamsMu.Unlock()
	setPlayback(cameraName, "play", detail)

	log.Info("play: looped preset started", "camera", cameraName, "preset", preset.Name)
	h.events.publish(event{Camera: cameraName, Action: "play", Text: detail, At: time.Now()})

	go func() {
		_ = cam.Stream(stdout)
		stopStream(cameraName)
		clearPlayback(cameraName)
		log.Info("play: looped preset ended", "camera", cameraName, "preset", preset.Name)
	}()
	go func() {
		_ = cmd.Wait()
		stopStream(cameraName)
	}()

	return t, nil
}

// SpeakForMQTT is called by the MQTT subscriber.
func (h *Handlers) SpeakForMQTT(cams []string, text, preset, voice string, loop bool) {
	var wg sync.WaitGroup
	for _, cam := range cams {
		wg.Add(1)
		go func(c string) {
			defer wg.Done()

			if preset != "" {
				_, _ = h.playPreset(h.log, c, "", preset, 3.0, loop)
			} else if text != "" {
				_, _ = h.speakText(h.log, c, text, voice, 3.0)
			}
		}(cam)
	}

	wg.Wait()
}

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
	if err := validateRequestGain(req.Gain); err != nil {
		return err
	}

	result := h.broadcastToCameras(c.Request().Context(), log, h.reg.Names(), req)
	status := http.StatusOK
	if len(result.Errors) > 0 {
		status = http.StatusMultiStatus
	}
	return c.JSON(status, result)
}

// --- Internal helpers ---

// resolveGainValue returns the numeric gain for a camera call. If reqGain >= 0
// it wins; otherwise the camera's current runtime gain is used; otherwise the
// default (3.0) is returned. Shared by gainForCall and effectiveGain.
func (h *Handlers) resolveGainValue(camera string, reqGain float64) float64 {
	if reqGain >= 0 {
		return reqGain
	}
	if gc := h.reg.GetGain(camera); gc != nil {
		return gc.Get()
	}
	return 3.0
}

// gainForCall returns a GainController for a one-off audio send. If reqGain >= 0
// that value is used; otherwise the camera's current runtime gain controller is
// returned (so runtime volume changes apply per-chunk during playback).
func (h *Handlers) gainForCall(camera string, reqGain float64) *cameras.GainController {
	if gc := h.reg.GetGain(camera); gc != nil && (reqGain < 0 || reqGain == gc.Get()) {
		return gc
	}
	if reqGain >= 0 {
		return cameras.NewGainController(reqGain)
	}
	return cameras.NewGainController(3.0)
}

// effectiveGain returns the numeric gain to use for ffmpeg-based paths (streams
// and looped presets). If reqGain >= 0 it wins; otherwise the camera's current
// runtime gain is used.
func (h *Handlers) effectiveGain(camera string, reqGain float64) float64 {
	return h.resolveGainValue(camera, reqGain)
}

func (h *Handlers) speakTextContext(
	ctx context.Context,
	log *clog.Logger,
	cameraName, text, voice string,
	gain float64,
) (*StepTimings, error) {
	t := NewStepTimings(3)
	cam, err := h.reg.GetForPlayback(cameraName)
	if err != nil {
		return t, err
	}
	op, err := h.beginPlaybackOperation(ctx, cameraName, cam, "speak", text)
	if err != nil {
		return t, err
	}
	defer op.finish()
	cfg := h.configSnapshot()

	if voice == "" {
		voice = cfg.TTS.DefaultVoice
	}

	// Gain is now applied at send time via GainController (per-chunk).
	// Transcode at unity so the raw file is clean; volume is adjusted live.

	if canStreamSpeech(op, cfg) {
		if err := h.streamSpeech(op, cfg, text, voice, h.gainForCall(cameraName, gain), t, nil); err != nil {
			return t, err
		}
	} else {
		ttsStart := time.Now()
		wav, err := h.ttsClient().SpeakContext(op.ctx, text, voice)
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
		rawPath, err := wavBytesToRawWithPrimeContext(op.ctx, wav, h.tmpDir, 1.0, cfg.PrimeSilenceMs)
		if err != nil {
			return t, fmt.Errorf("transcoding: %w", err)
		}
		t.Add("transcode_ms", transcodeStart)
		defer os.Remove(rawPath)

		log.Debug("speak: sending to camera", "camera", cameraName)
		sendTiming, err := op.send(rawPath, h.gainForCall(cameraName, gain))
		if err != nil {
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

	}
	h.events.publish(
		event{
			Camera: cameraName, Action: "speak", Text: text, Voice: voice, At: time.Now(),
			Replay: playbackReplay(
				"/api/speak",
				map[string]any{"camera": cameraName, "text": text, "voice": voice},
				gain,
			),
		},
	)

	return t, nil
}

func (h *Handlers) playPresetContext(
	ctx context.Context,
	log *clog.Logger,
	cameraName, category, presetName string,
	gain float64,
	loop int,
) (*StepTimings, error) {
	t := NewStepTimings(3)
	cam, err := h.reg.GetForPlayback(cameraName)
	if err != nil {
		return t, err
	}
	op, err := h.beginPlaybackOperation(
		context.WithoutCancel(ctx),
		cameraName,
		cam,
		"play",
		presetName,
	)
	if err != nil {
		return t, err
	}
	stopRequest := context.AfterFunc(ctx, op.cancel)
	streamStarted := false
	defer func() {
		stopRequest()
		if !streamStarted {
			op.finish()
		}
	}()
	if ctx.Err() != nil {
		return t, ctx.Err()
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
	replay := playbackReplay(
		"/api/play",
		map[string]any{
			"camera":   cameraName,
			"preset":   preset.Name,
			"category": preset.Category,
			"loop":     loop,
		},
		gain,
	)
	// Record the original preset request once; applying preset gain must not alter replay gain.
	publish := func() {
		h.events.publish(
			event{
				Camera: cameraName,
				Action: "play",
				Text:   preset.Name,
				At:     time.Now(),
				Replay: replay,
			},
		)
	}

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
		if !cameras.CanLiveStream(cam) {
			return t, cameras.ErrLiveStreamUnsupported
		}
		streamURL, resolveErr := resolveStreamURLContext(op.ctx, preset.URL)
		if resolveErr != nil {
			return t, resolveErr
		}
		err = h.startPreparedStream(log, op, streamURL, preset.URL, gain)
		if err != nil {
			return t, err
		}
		streamStarted = true
		publish()
		log.Info(
			"play: stream preset started",
			"camera",
			cameraName,
			"preset",
			preset.Name,
			"url",
			preset.URL,
		)
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
		result, loopErr := h.playPresetLooped(log, op, preset, sendPath, t, gain, loop)
		if loopErr == nil {
			publish()
		}
		streamStarted = loopErr == nil
		return result, loopErr
	}
	cfg := h.configSnapshot()

	// Prepend prime silence to warm the camera's audio engine.
	// (Only for the non-looped SendRaw path; looped presets use adelay.)
	if cfg.PrimeSilenceMs > 0 {
		primed, err := prependSilenceToNewFile(sendPath, h.tmpDir, cfg.PrimeSilenceMs)
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

	sendTiming, err := op.send(sendPath, h.gainForCall(cameraName, gain))
	if err != nil {
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
	publish()

	return t, nil
}

// playPresetLooped plays a preset in a loop using ffmpeg -stream_loop,
// piped to cam.Stream(). loop=-1 means infinite; loop=N (N>0) plays the
// preset N+1 times (once + N loops). This registers a streamSession
// so the loop can be paused, resumed, and stopped just like a live stream.
func (h *Handlers) playPresetLooped(
	log *clog.Logger,
	op *playbackOperation,
	preset *library.Preset,
	rawPath string,
	t *StepTimings,
	gain float64,
	loop int,
) (*StepTimings, error) {
	cam, cameraName := op.cam, op.camera
	if !cameras.CanLiveStream(cam) {
		return t, cameras.ErrLiveStreamUnsupported
	}
	ctx, cancel := context.WithCancel(op.ctx)
	cfg := h.configSnapshot()

	// Build audio filter: volume applies the requested/camera gain, and
	// adelay adds prime silence at the start of each loop iteration.
	g := 1.0
	af := fmt.Sprintf("volume=%.2f", g)
	if cfg.PrimeSilenceMs > 0 {
		af = fmt.Sprintf("adelay=%d|%d,volume=%.2f", cfg.PrimeSilenceMs, cfg.PrimeSilenceMs, g)
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
		op.finish()
		return t, fmt.Errorf("ffmpeg stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		op.finish()
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
		op:      op,
	}
	operationsMu.Lock()
	if operations[cameraName] != op || ctx.Err() != nil {
		operationsMu.Unlock()
		cancel()
		_ = cmd.Wait()
		op.finish()
		return t, context.Canceled
	}
	activeStreamsMu.Lock()
	activeStreams[cameraName] = session
	activeStreamsMu.Unlock()
	setPlayback(cameraName, "play", detail)
	operationsMu.Unlock()

	log.Info("play: looped preset started", "camera", cameraName, "preset", preset.Name)

	// Wrap stdout with a level tap so the VU meter can sample audio levels.
	tap := &levelTapReader{r: stdout, session: session, gain: h.gainForCall(cameraName, gain)}

	go func() {
		defer finishStream(cameraName, session)
		streamErr := cameras.StreamContext(ctx, cam, tap)
		cancel()
		_ = cmd.Wait()
		elapsed := time.Since(session.started)
		if streamErr != nil {
			log.Warn("play: looped preset ended with error",
				"camera", cameraName, "preset", preset.Name,
				"elapsed", elapsed, "err", streamErr)
		} else {
			log.Info("play: looped preset ended",
				"camera", cameraName, "preset", preset.Name, "elapsed", elapsed)
		}
	}()

	return t, nil
}

// BroadcastToCameras sends text or preset to multiple cameras in parallel.
// Used by the broadcast endpoint and MCP broadcast tool.
func (h *Handlers) BroadcastToCameras(
	cams []string,
	text, preset, voice string,
	loop int,
) BroadcastResult {
	return h.BroadcastToCamerasContext(context.Background(), cams, text, preset, voice, loop)
}

// BroadcastResult preserves successes and failures consistently for REST and MCP.
type BroadcastResult struct {
	Status    string   `json:"status"`
	Succeeded []string `json:"succeeded"`
	Errors    []string `json:"errors,omitempty"`
}

// BroadcastToCamerasContext prepares common speech once and returns all camera results.
func (h *Handlers) BroadcastToCamerasContext(
	ctx context.Context,
	cams []string,
	text, preset, voice string,
	loop int,
) BroadcastResult {
	return h.broadcastToCameras(
		ctx,
		h.log,
		cams,
		broadcastReq{Text: text, Preset: preset, Voice: voice, Loop: loop},
	)
}

func (h *Handlers) broadcastToCameras(
	ctx context.Context,
	log *clog.Logger,
	names []string,
	req broadcastReq,
) BroadcastResult {
	result := BroadcastResult{Status: "ok", Succeeded: []string{}}
	var mu sync.Mutex
	record := func(camera string, err error) {
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			result.Status = "partial_failure"
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", camera, err))
		} else {
			result.Succeeded = append(result.Succeeded, camera)
		}
	}
	// Presets already share prepared audio on disk. Streaming and loops retain
	// their ordinary per-camera lifecycle; they do not synthesize or transcode TTS.
	if req.Preset != "" {
		var wg sync.WaitGroup
		for _, name := range names {
			wg.Go(func() {
				_, err := h.playPresetContext(
					ctx,
					log,
					name,
					req.Category,
					req.Preset,
					requestGain(req.Gain),
					req.Loop,
				)
				record(name, err)
			})
		}
		wg.Wait()
		return result
	}
	ops := make([]*playbackOperation, 0, len(names))
	for _, name := range names {
		cam, err := h.reg.GetForPlayback(name)
		if err != nil {
			record(name, err)
			continue
		}
		op, err := h.beginPlaybackOperation(ctx, name, cam, "speak", req.Text)
		if err != nil {
			record(name, err)
			continue
		}
		ops = append(ops, op)
		defer op.finish()
	}
	if len(ops) == 0 {
		return result
	}
	prepareCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	remaining := len(ops)
	var prepareMu sync.Mutex
	for _, op := range ops {
		stop := context.AfterFunc(op.ctx, func() {
			prepareMu.Lock()
			defer prepareMu.Unlock()
			remaining--
			if remaining == 0 {
				cancel()
			}
		})
		defer stop()
	}
	cfg := h.configSnapshot()
	voice := req.Voice
	if voice == "" {
		voice = cfg.TTS.DefaultVoice
	}
	wav, err := h.ttsClient().SpeakContext(prepareCtx, req.Text, voice)
	var raw string
	if err == nil {
		raw, err = wavBytesToRawWithPrimeContext(prepareCtx, wav, h.tmpDir, 1, cfg.PrimeSilenceMs)
	}
	if err != nil {
		for _, op := range ops {
			record(op.camera, err)
		}
		return result
	}
	defer os.Remove(raw)
	var wg sync.WaitGroup
	for _, op := range ops {
		wg.Go(func() {
			_, err := op.send(raw, h.gainForCall(op.camera, requestGain(req.Gain)))
			record(op.camera, err)
			if err == nil {
				h.events.publish(
					event{
						Camera: op.camera,
						Action: "speak",
						Text:   req.Text,
						Voice:  voice,
						At:     time.Now(),
						Replay: playbackReplay(
							"/api/speak",
							map[string]any{"camera": op.camera, "text": req.Text, "voice": voice},
							requestGain(req.Gain),
						),
					},
				)
			}
		})
	}
	wg.Wait()
	return result
}

package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/tts"
	"github.com/labstack/echo/v4"
)

type speakerBenchmarkRequest struct {
	Preset          string `json:"preset"`
	Camera          string `json:"camera"`
	Text            string `json:"text"`
	Voice           string `json:"voice"`
	SampleRate      int    `json:"sample_rate"`
	Channels        int    `json:"channels"`
	ConfirmPlayback bool   `json:"confirm_playback"`
	StreamingFirst  bool   `json:"streaming_first"`
}

// StartSpeakerBenchmark compares actual camera delivery without changing saved configuration.
func (h *Handlers) StartSpeakerBenchmark(c echo.Context) error {
	var req speakerBenchmarkRequest
	if err := c.Bind(&req); err != nil || !req.ConfirmPlayback || req.Camera == "" ||
		strings.TrimSpace(req.Text) == "" ||
		len(req.Text) > 2000 ||
		req.SampleRate < 8000 ||
		req.SampleRate > 96000 ||
		req.Channels < 1 ||
		req.Channels > 2 {
		return echo.NewHTTPError(
			http.StatusBadRequest,
			"confirm playback and provide camera, preset, text (1–2000 bytes), PCM rate (8000–96000) and channels (1–2)",
		)
	}
	presets, err := config.ListTTSPresets(h.db)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "could not load presets")
	}
	cfg := h.configSnapshot()
	found := false
	for _, preset := range presets {
		if preset.Name == req.Preset {
			cfg.TTS = config.TTSConfig{
				URL:           preset.Endpoint,
				Model:         preset.Model,
				APIKey:        preset.APIKey,
				DefaultVoice:  preset.DefaultVoice,
				PCMSampleRate: req.SampleRate,
				PCMChannels:   req.Channels,
			}
			found = true
			break
		}
	}
	if !found {
		return echo.NewHTTPError(http.StatusNotFound, "TTS preset not found")
	}
	cam, err := h.reg.GetForPlayback(req.Camera)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	if _, ok := cam.(speechStreamer); !ok {
		return echo.NewHTTPError(
			http.StatusBadRequest,
			"speaker comparison requires a streaming-capable Hikvision camera",
		)
	}
	workerCtx, ok := h.describes.worker.acquire()
	if !ok {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "audio worker busy; try again shortly")
	}
	ctx, cancel := context.WithTimeout(workerCtx, 10*time.Minute)
	op, err := h.beginPlaybackOperation(ctx, req.Camera, cam, "tts-benchmark", "Speaker comparison")
	if err != nil {
		cancel()
		h.describes.worker.release()
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	}
	id := h.describes.create(req.Camera)
	h.describes.mu.Lock()
	h.describes.jobs[id].cancel = cancel
	h.describes.mu.Unlock()
	h.describes.update(id, "tts", map[string]any{"camera": req.Camera, "preset": req.Preset})
	gain := cameras.NewGainController(h.gainForCall(req.Camera, -1).Get())
	log := h.logger(c)
	log.Info("speaker benchmark starting", "job_id", id, "camera", req.Camera,
		"preset", req.Preset, "model", cfg.TTS.Model, "pcm_rate", cfg.TTS.PCMSampleRate,
		"pcm_channels", cfg.TTS.PCMChannels, "streaming_first", req.StreamingFirst)
	go func() {
		defer h.describes.worker.release()
		defer cancel()
		defer op.finish()
		result, runErr := h.runSpeakerComparison(
			op,
			cfg,
			req,
			gain,
			func(stage string, result map[string]any) {
				log.Info("speaker benchmark progress", "job_id", id, "camera", req.Camera,
					"mode", result["mode"], "stage", stage, "timings", result["timings"])
				h.describes.update(id, stage, result)
			},
		)
		if runErr != nil {
			log.Error("speaker benchmark failed", "camera", req.Camera, "err", runErr)
		}
		h.describes.finish(id, result, runErr)
	}()
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.JSON(http.StatusAccepted, h.describes.get(id))
}

func (h *Handlers) runSpeakerComparison(
	op *playbackOperation,
	cfg config.Config,
	req speakerBenchmarkRequest,
	gain *cameras.GainController,
	progress func(string, map[string]any),
) (map[string]any, error) {
	voice := req.Voice
	if voice == "" {
		voice = cfg.TTS.DefaultVoice
	}
	modes := []string{"buffered", "streaming"}
	if req.StreamingFirst {
		modes = []string{"streaming", "buffered"}
	}
	result := map[string]any{
		"camera": req.Camera,
		"preset": req.Preset,
		"gain":   gain.Get(),
		"order":  modes,
	}
	var runErr error
	for _, mode := range modes {
		if runErr = op.ctx.Err(); runErr != nil {
			break
		}
		operationsMu.Lock()
		if operations[op.camera] == op {
			op.detail = mode + " speaker comparison"
			setPlayback(op.camera, op.source, op.detail)
			playbackStatesMu.Lock()
			playbackStates[op.camera].State = "preparing"
			playbackStatesMu.Unlock()
		}
		operationsMu.Unlock()
		start := time.Now()
		timings := NewStepTimings(4)
		report := func(stage string) { result["mode"] = mode; result["timings"] = timings.Ms(); progress(stage, result) }
		report("tts")
		var firstAudio int64
		if mode == "streaming" {
			runErr = h.streamSpeech(op, cfg, req.Text, voice, gain, timings, func(stage string) {
				if stage == "playing" {
					firstAudio = TotalMs(start)
				}
				report(stage)
			})
		} else {
			runErr = h.bufferedBenchmarkSpeech(op, cfg, req.Text, voice, gain, timings, func(stage string) {
				if stage == "playing" {
					firstAudio = TotalMs(start)
				}
				report(stage)
			})
		}
		row := map[string]any{"tts_mode": mode, "timings": timings.Ms(), "total_ms": TotalMs(start)}
		if firstAudio > 0 {
			row["ttfs_ms"] = firstAudio
		}
		if runErr != nil {
			row["error"] = runErr.Error()
		}
		result[mode] = row
		if runErr != nil {
			break
		}
		h.events.publish(
			event{
				Camera: req.Camera,
				Action: "tts-benchmark",
				Text:   req.Text,
				Voice:  voice,
				At:     time.Now(),
			},
		)
	}
	if op.ctx.Err() != nil {
		runErr = op.ctx.Err()
	}
	return result, runErr
}

func (h *Handlers) bufferedBenchmarkSpeech(
	op *playbackOperation,
	cfg config.Config,
	text, voice string,
	gain *cameras.GainController,
	timings *StepTimings,
	report func(string),
) error {
	start := time.Now()
	wav, err := tts.NewClient(cfg.TTS.URL, cfg.TTS.Model, cfg.TTS.APIKey).
		SpeakContext(op.ctx, text, voice)
	if err != nil {
		return err
	}
	timings.Add("tts_ms", start)
	report("transcode")
	start = time.Now()
	path, err := wavBytesToRawWithPrimeContext(op.ctx, wav, h.tmpDir, 1, cfg.PrimeSilenceMs)
	if err != nil {
		return err
	}
	defer os.Remove(path)
	timings.Add("transcode_ms", start)
	report("connecting")
	start = time.Now()
	op.onPlaying = func() { timings.Add("send_open_ms", start); report("playing") }
	timing, err := op.send(path, gain)
	timings.steps["send_playback_ms"] = time.Duration(timing.PlaybackMs) * time.Millisecond
	if err != nil {
		return fmt.Errorf("camera delivery: %w", err)
	}
	return nil
}

// CancelSpeakerBenchmark cancels only this job, never a replacement camera action.
func (h *Handlers) CancelSpeakerBenchmark(c echo.Context) error {
	h.describes.mu.Lock()
	defer h.describes.mu.Unlock()
	job := h.describes.jobs[c.Param("id")]
	if job == nil || job.cancel == nil {
		return echo.NewHTTPError(http.StatusNotFound, "speaker benchmark not found")
	}
	job.cancel()
	return c.JSON(http.StatusOK, map[string]string{"status": "canceling"})
}

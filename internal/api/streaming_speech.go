package api

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"time"

	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/tts"
)

type speechStreamer interface {
	StreamWithGainContext(context.Context, io.Reader, *cameras.GainController) error
}

func canStreamSpeech(op *playbackOperation, cfg config.Config) bool {
	_, supported := op.cam.(speechStreamer)
	return cfg.TTS.Streaming && supported
}

// streamSpeech keeps generation, conversion and delivery connected without a temporary file.
// Failures never replay the message: some audio may already have reached the speaker.
func (h *Handlers) streamSpeech(op *playbackOperation, cfg config.Config, text, voice string,
	gain *cameras.GainController, timings *StepTimings, report func(string),
) error {
	ctx, cancel := context.WithTimeout(op.ctx, 5*time.Minute)
	defer cancel()
	stage := func(name string) {
		if report != nil {
			report(name)
		}
	}
	start := time.Now()
	client := tts.NewClient(cfg.TTS.URL, cfg.TTS.Model, cfg.TTS.APIKey)
	body, err := client.OpenPCM(ctx, text, voice)
	if err != nil {
		return err
	}
	defer body.Close()
	timings.Add("tts_ms", start)
	stage("transcode")
	start = time.Now()
	rate, channels := cfg.TTS.PCMSampleRate, cfg.TTS.PCMChannels
	if rate == 0 {
		rate = 24000
	}
	if channels == 0 {
		channels = 1
	}
	cmd := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-hide_banner",
		"-loglevel",
		"error",
		"-probesize",
		"32",
		"-analyzeduration",
		"0",
		"-f",
		"s16le",
		"-ar",
		strconv.Itoa(rate),
		"-ac",
		strconv.Itoa(channels),
		"-i",
		"pipe:0",
		"-ar",
		"8000",
		"-ac",
		"1",
		"-f",
		"mulaw",
		"-flush_packets",
		"1",
		"pipe:1",
	)
	cmd.Stdin = body
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("streaming converter: %w", err)
	}
	if err = cmd.Start(); err != nil {
		return fmt.Errorf("starting streaming converter: %w", err)
	}
	defer func() { cancel(); body.Close(); stdout.Close(); _ = cmd.Wait() }()
	reader := bufio.NewReader(stdout)
	if _, err = reader.Peek(1); err != nil {
		return fmt.Errorf("streaming converter produced no audio: %w", err)
	}
	timings.Add("transcode_ms", start)
	stage("connecting")
	sendStart := time.Now()
	op.onPlaying = func() { timings.Add("send_open_ms", sendStart); stage("playing") }
	prime := bytes.Repeat([]byte{0xff}, max(0, cfg.PrimeSilenceMs)*8)
	_, err = op.sendWith(gain, func(gc *cameras.GainController) (cameras.SendTiming, error) {
		err := op.cam.(speechStreamer).StreamWithGainContext(
			ctx,
			io.MultiReader(bytes.NewReader(prime), reader),
			gc,
		)
		return cameras.SendTiming{}, err
	})
	if open, ok := timings.steps["send_open_ms"]; ok {
		timings.steps["send_playback_ms"] = time.Since(sendStart) - open
	}
	if err != nil {
		return fmt.Errorf("streaming camera audio: %w", err)
	}
	if err = cmd.Wait(); err != nil {
		return fmt.Errorf("streaming converter failed: %w", err)
	}
	return nil
}

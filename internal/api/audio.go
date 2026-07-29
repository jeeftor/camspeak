package api

import (
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	clog "github.com/charmbracelet/log"
	"github.com/labstack/echo/v4"

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
				err = h.playPreset(log, cam, req.Category, req.Preset, req.Gain)
			} else {
				err = h.speakText(log, cam, req.Text, req.Voice, req.Gain)
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

func (h *Handlers) speakText(log *clog.Logger, cameraName, text, voice string, gain float64) error {
	cam, err := h.reg.Get(cameraName)
	if err != nil {
		return err
	}

	if voice == "" {
		voice = h.cfg.TTS.DefaultVoice
	}

	if gain <= 0 {
		gain = 3.0 // default boost
	}

	ttsStart := time.Now()
	wav, err := h.tts.Speak(text, voice)
	if err != nil {
		return fmt.Errorf("TTS: %w", err)
	}
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

	rawPath, err := wavBytesToRaw(wav, h.tmpDir, gain)
	if err != nil {
		return fmt.Errorf("transcoding: %w", err)
	}
	defer os.Remove(rawPath)

	log.Debug("speak: sending to camera", "camera", cameraName)
	sendStart := time.Now()
	if err := cam.SendRaw(rawPath); err != nil {
		return fmt.Errorf("sending to camera: %w", err)
	}
	log.Debug(
		"speak: camera send complete",
		"camera",
		cameraName,
		"elapsed",
		time.Since(sendStart),
	)

	h.events.publish(event{Camera: cameraName, Action: "speak", Text: text, At: time.Now()})

	return nil
}

func (h *Handlers) playPreset(
	log *clog.Logger,
	cameraName, category, presetName string,
	gain float64,
) error {
	cam, err := h.reg.Get(cameraName)
	if err != nil {
		return err
	}

	var preset *library.Preset
	if category != "" {
		preset, err = h.store.Get(category, presetName)
	} else {
		preset, err = h.store.GetByName(presetName)
	}

	if err != nil {
		return err
	}

	// If gain is specified, re-transcode the raw file with the gain filter.
	// The stored raw is already G.711ulaw 8kHz, so we read it as mulaw and
	// apply volume, then output mulaw again.
	sendPath := preset.RawPath
	if gain > 0 && gain != 3.0 {
		boosted, err := boostRawGain(preset.RawPath, h.tmpDir, gain)
		if err != nil {
			log.Warn("play: gain boost failed, sending original", "err", err)
		} else {
			defer os.Remove(boosted)
			sendPath = boosted
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
	)
	sendStart := time.Now()
	if err := cam.SendRaw(sendPath); err != nil {
		return fmt.Errorf("sending to camera: %w", err)
	}
	log.Debug(
		"play: camera send complete",
		"camera",
		cameraName,
		"elapsed",
		time.Since(sendStart),
	)

	h.events.publish(event{Camera: cameraName, Action: "play", Text: preset.Name, At: time.Now()})

	return nil
}

// SpeakForMQTT is called by the MQTT subscriber.
func (h *Handlers) SpeakForMQTT(cams []string, text, preset, voice string) {
	var wg sync.WaitGroup
	for _, cam := range cams {
		wg.Add(1)
		go func(c string) {
			defer wg.Done()

			if preset != "" {
				h.playPreset(h.log, c, "", preset, 3.0) //nolint:errcheck
			} else if text != "" {
				h.speakText(h.log, c, text, voice, 3.0) //nolint:errcheck
			}
		}(cam)
	}

	wg.Wait()
}

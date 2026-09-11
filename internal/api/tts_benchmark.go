package api

import (
	"net/http"
	"strings"

	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/tts"
	"github.com/labstack/echo/v4"
)

// BenchmarkTTS tests a saved endpoint/model without activating it or playing on a camera.
func (h *Handlers) BenchmarkTTS(c echo.Context) error {
	var req struct {
		Preset     string `json:"preset"`
		Mode       string `json:"mode"`
		Text       string `json:"text"`
		Voice      string `json:"voice"`
		SampleRate int    `json:"sample_rate"`
		Channels   int    `json:"channels"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	if strings.TrimSpace(req.Text) == "" || len(req.Text) > 2000 || (req.Mode != "buffered" && req.Mode != "streaming") || req.SampleRate < 8000 || req.SampleRate > 96000 || req.Channels < 1 || req.Channels > 2 {
		return echo.NewHTTPError(http.StatusBadRequest, "provide text (1–2000 bytes), mode, sample_rate (8000–96000), and channels (1–2)")
	}
	presets, err := config.ListTTSPresets(h.db)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "could not load TTS presets")
	}
	for _, preset := range presets {
		if preset.Name != req.Preset {
			continue
		}
		voice := req.Voice
		if voice == "" {
			voice = preset.DefaultVoice
		}
		client := tts.NewClient(preset.Endpoint, preset.Model, preset.APIKey)
		result, err := client.Benchmark(c.Request().Context(), req.Text, voice, req.Mode, req.SampleRate, req.Channels)
		if err != nil {
			h.logger(c).Error("TTS benchmark failed", "preset", req.Preset, "mode", req.Mode, "err", err)
			return echo.NewHTTPError(http.StatusBadGateway, err.Error())
		}
		c.Response().Header().Set("Cache-Control", "no-store")
		return c.JSON(http.StatusOK, result)
	}
	return echo.NewHTTPError(http.StatusNotFound, "TTS preset not found")
}

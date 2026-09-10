package api

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/tts"
)

// ListTTSPresets handles GET /api/config/tts — returns all TTS presets.
func (h *Handlers) ListTTSPresets(c echo.Context) error {
	presets, err := config.ListTTSPresets(h.db)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	for i := range presets {
		presets[i] = presets[i].Sanitized()
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"presets": presets,
		"active":  h.configSnapshot().TTS.Sanitized(),
	})
}

// CreateTTSPreset handles POST /api/config/tts — creates a new TTS preset.
func (h *Handlers) CreateTTSPreset(c echo.Context) error {
	h.configEditMu.Lock()
	defer h.configEditMu.Unlock()
	var p config.TTSPreset
	if err := c.Bind(&p); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	if p.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if p.Endpoint == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "endpoint is required")
	}
	if err := config.SaveTTSPreset(h.db, p); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := h.reloadTTS(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusCreated, p.Sanitized())
}

// UpdateTTSPreset handles PUT /api/config/tts/:name — updates an existing TTS preset.
func (h *Handlers) UpdateTTSPreset(c echo.Context) error {
	h.configEditMu.Lock()
	defer h.configEditMu.Unlock()
	name := c.Param("name")
	var p config.TTSPreset
	if err := c.Bind(&p); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	p.Name = name
	if p.Endpoint == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "endpoint is required")
	}
	// Editing an active preset must not silently deactivate it.
	presets, err := config.ListTTSPresets(h.db)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	for _, existing := range presets {
		if existing.Name == name {
			if existing.IsActive {
				p.IsActive = true
			}
			if p.APIKey == "" && !p.ClearAPIKey {
				p.APIKey = existing.APIKey
			}
		}
	}
	if err := config.SaveTTSPreset(h.db, p); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if err := h.reloadTTS(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, p.Sanitized())
}

// DeleteTTSPreset handles DELETE /api/config/tts/:name — deletes a TTS preset.
func (h *Handlers) DeleteTTSPreset(c echo.Context) error {
	h.configEditMu.Lock()
	defer h.configEditMu.Unlock()
	name := c.Param("name")
	if err := config.DeleteTTSPreset(h.db, name); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"deleted": name})
}

// ActivateTTSPreset handles POST /api/config/tts/:name/activate — sets the active TTS preset.
func (h *Handlers) ActivateTTSPreset(c echo.Context) error {
	h.configEditMu.Lock()
	defer h.configEditMu.Unlock()
	name := c.Param("name")
	if err := config.SetActiveTTSPreset(h.db, name); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	if err := h.reloadTTS(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"active": name})
}

// reloadTTS publishes configuration and its immutable authenticated client together.
func (h *Handlers) reloadTTS() error {
	presets, err := config.ListTTSPresets(h.db)
	if err != nil {
		return err
	}
	h.cfgMu.Lock()
	for _, p := range presets {
		if p.IsActive {
			h.cfg.TTS = config.TTSConfig{
				URL:          p.Endpoint,
				Model:        p.Model,
				DefaultVoice: p.DefaultVoice,
				APIKey:       p.APIKey,
			}
			config.ApplyEnvOverrides(h.cfg)
			h.tts = tts.NewClient(h.cfg.TTS.URL, h.cfg.TTS.Model, h.cfg.TTS.APIKey)
			break
		}
	}
	h.cfgMu.Unlock()
	return nil
}

// ListVisionPrompts handles GET /api/config/vision-prompts — returns all saved vision prompts.
func (h *Handlers) ListVisionPrompts(c echo.Context) error {
	prompts, err := config.ListVisionPrompts(h.db)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, prompts)
}

// CreateVisionPrompt handles POST /api/config/vision-prompts — creates or updates a vision prompt.
func (h *Handlers) CreateVisionPrompt(c echo.Context) error {
	var p config.VisionPrompt
	if err := c.Bind(&p); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	if p.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name is required")
	}
	if err := config.SaveVisionPrompt(h.db, p); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.logger(c).Info("vision prompt saved", "name", p.Name)
	return c.JSON(http.StatusCreated, p)
}

// DeleteVisionPrompt handles DELETE /api/config/vision-prompts/:name — removes a vision prompt.
func (h *Handlers) DeleteVisionPrompt(c echo.Context) error {
	name := c.Param("name")
	if err := config.DeleteVisionPrompt(h.db, name); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	h.logger(c).Info("vision prompt deleted", "name", name)
	return c.JSON(http.StatusOK, map[string]string{"deleted": name})
}

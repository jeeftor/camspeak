package api

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/jeeftor/camspeak/internal/config"
)

// ListTTSPresets handles GET /api/config/tts — returns all TTS presets.
func (h *Handlers) ListTTSPresets(c echo.Context) error {
	presets, err := config.ListTTSPresets(h.db)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]interface{}{
		"presets": presets,
		"active":  h.cfg.TTS,
	})
}

// CreateTTSPreset handles POST /api/config/tts — creates a new TTS preset.
func (h *Handlers) CreateTTSPreset(c echo.Context) error {
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
	return c.JSON(http.StatusCreated, p)
}

// UpdateTTSPreset handles PUT /api/config/tts/:name — updates an existing TTS preset.
func (h *Handlers) UpdateTTSPreset(c echo.Context) error {
	name := c.Param("name")
	var p config.TTSPreset
	if err := c.Bind(&p); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid JSON body")
	}
	p.Name = name
	if err := config.SaveTTSPreset(h.db, p); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, p)
}

// DeleteTTSPreset handles DELETE /api/config/tts/:name — deletes a TTS preset.
func (h *Handlers) DeleteTTSPreset(c echo.Context) error {
	name := c.Param("name")
	if err := config.DeleteTTSPreset(h.db, name); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"deleted": name})
}

// ActivateTTSPreset handles POST /api/config/tts/:name/activate — sets the active TTS preset.
func (h *Handlers) ActivateTTSPreset(c echo.Context) error {
	name := c.Param("name")
	if err := config.SetActiveTTSPreset(h.db, name); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	// Reload the active TTS config into the running config
	presets, err := config.ListTTSPresets(h.db)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
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
			break
		}
	}
	h.cfgMu.Unlock()
	return c.JSON(http.StatusOK, map[string]string{"active": name})
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

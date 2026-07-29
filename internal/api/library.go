package api

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
)

// ListLibrary handles GET /api/library.
func (h *Handlers) ListLibrary(c echo.Context) error {
	presets, err := h.store.List()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	// Filter out transient _tmp presets (created by ad-hoc speak, raw files
	// are deleted immediately but DB rows linger).
	filtered := presets[:0]
	for _, p := range presets {
		if p.Category != "_tmp" {
			filtered = append(filtered, p)
		}
	}

	return c.JSON(http.StatusOK, filtered)
}

// TTSPreview handles POST /api/tts/preview — generates TTS and returns WAV audio.
func (h *Handlers) TTSPreview(c echo.Context) error {
	var req struct {
		Text  string `json:"text"`
		Voice string `json:"voice"`
	}
	if err := c.Bind(&req); err != nil || req.Text == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "text required")
	}

	voice := req.Voice
	if voice == "" {
		voice = h.cfg.TTS.DefaultVoice
	}

	wav, err := h.tts.Speak(req.Text, voice)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("TTS failed: %s", err))
	}

	return c.Blob(http.StatusOK, "audio/wav", wav)
}

// GeneratePreset handles POST /api/library — TTS → save preset.
func (h *Handlers) GeneratePreset(c echo.Context) error {
	var req genPresetReq
	if err := c.Bind(&req); err != nil || req.Name == "" || req.Text == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name and text required")
	}

	if req.Category == "" {
		req.Category = "default"
	}

	voice := req.Voice
	if voice == "" {
		voice = h.cfg.TTS.DefaultVoice
	}

	wav, err := h.tts.Speak(req.Text, voice)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("TTS failed: %s", err))
	}

	preset, err := h.store.Save(req.Category, req.Name, req.Text, voice, wav)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, preset)
}

// UploadPreset handles POST /api/library/upload — audio file → save preset.
func (h *Handlers) UploadPreset(c echo.Context) error {
	name := c.FormValue("name")
	category := c.FormValue("category")

	if name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name required")
	}

	if category == "" {
		category = "uploads"
	}

	file, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "file required")
	}

	src, err := file.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer src.Close()

	// Sanitize filename for temp file pattern (strip path separators, wildcards)
	safeName := sanitizeFilename(file.Filename)

	tmp, err := os.CreateTemp(h.tmpDir, "camspeak_upload_*_"+safeName)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer os.Remove(tmp.Name())

	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()

		return echo.NewHTTPError(
			http.StatusInternalServerError,
			fmt.Sprintf("reading upload: %s", err),
		)
	}

	tmp.Close()

	preset, err := h.store.SaveFile(category, name, tmp.Name())
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, preset)
}

// DeletePreset handles DELETE /api/library/:category/:name.
func (h *Handlers) DeletePreset(c echo.Context) error {
	err := h.store.Delete(c.Param("category"), c.Param("name"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	return c.NoContent(http.StatusNoContent)
}

// RenamePreset handles PATCH /api/library/:category/:name.
func (h *Handlers) RenamePreset(c echo.Context) error {
	var body struct {
		Name     string `json:"name"`
		Category string `json:"category"`
	}
	if err := c.Bind(&body); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	if body.Name == "" && body.Category == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "must provide name or category")
	}

	preset, err := h.store.Rename(c.Param("category"), c.Param("name"), body.Category, body.Name)
	if err != nil {
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	}

	return c.JSON(http.StatusOK, preset)
}

// PreviewPreset handles GET /api/library/:category/:name/preview — streams WAV.
func (h *Handlers) PreviewPreset(c echo.Context) error {
	preset, err := h.store.Get(c.Param("category"), c.Param("name"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	// Convert raw → WAV on the fly for browser preview
	wav, err := rawToWAV(preset.RawPath, h.tmpDir)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer os.Remove(wav)

	return c.File(wav)
}

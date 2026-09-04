package api

import (
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"time"

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

	start := time.Now()
	wav, err := h.tts.Speak(req.Text, voice)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("TTS failed: %s", err))
	}

	c.Response().Header().Set("X-TTS-Ms", fmt.Sprintf("%d", time.Since(start).Milliseconds()))
	return c.Blob(http.StatusOK, "audio/wav", wav)
}

// GeneratePreset handles POST /api/library — TTS → save preset, or save a
// stream URL as a preset. When the request includes a "url" field, a stream
// preset is created (no TTS, no raw file). Otherwise a TTS clip is generated
// and saved as a raw audio file.
func (h *Handlers) GeneratePreset(c echo.Context) error {
	var req genPresetReq
	if err := c.Bind(&req); err != nil || req.Name == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "name required")
	}

	// Stream preset: URL provided, no TTS needed.
	if req.URL != "" {
		return h.createStreamPreset(c, req)
	}

	// Audio preset: TTS text required.
	if req.Text == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "text or url required")
	}

	if req.Category == "" {
		req.Category = "default"
	}

	voice := req.Voice
	if voice == "" {
		voice = h.cfg.TTS.DefaultVoice
	}

	start := time.Now()
	t := NewStepTimings(2)

	ttsStart := time.Now()
	wav, err := h.tts.Speak(req.Text, voice)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, fmt.Sprintf("TTS failed: %s", err))
	}
	t.Add("tts_ms", ttsStart)

	saveStart := time.Now()
	preset, err := h.store.Save(req.Category, req.Name, req.Text, voice, wav)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	t.Add("save_ms", saveStart)

	// preset is a value, not pointer, after Save returns — build response
	return c.JSON(http.StatusCreated, map[string]any{
		"name":     preset.Name,
		"category": preset.Category,
		"text":     preset.Text,
		"voice":    preset.Voice,
		"duration": preset.Duration,
		"size":     preset.Size,
		"created":  preset.Created,
		"timings":  t.Ms(),
		"total_ms": TotalMs(start),
	})
}

// createStreamPreset saves a live stream URL as a named preset.
func (h *Handlers) createStreamPreset(c echo.Context, req genPresetReq) error {
	log := h.logger(c)

	// Validate URL scheme (http/https only).
	parsed, err := neturl.Parse(req.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return echo.NewHTTPError(http.StatusBadRequest, "url must be http or https")
	}

	if req.Category == "" {
		req.Category = "streams"
	}

	preset, err := h.store.SaveStream(req.Category, req.Name, req.URL)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	log.Info("library: stream preset saved", "name", preset.Name, "category", preset.Category)
	return c.JSON(http.StatusCreated, map[string]any{
		"name":     preset.Name,
		"category": preset.Category,
		"url":      preset.URL,
		"created":  preset.Created,
	})
}

// UploadPreset handles POST /api/library/upload — audio file → save preset.
// The file upload (HTTP transfer) completes before this handler runs (Echo
// parses the multipart body first). The handler saves the temp file, starts
// ffmpeg transcoding in a goroutine, and returns a job ID immediately so the
// client can poll GET /api/library/upload/jobs/:id for transcoding progress.
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
	tmpName := tmp.Name()

	if _, err := io.Copy(tmp, src); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return echo.NewHTTPError(
			http.StatusInternalServerError,
			fmt.Sprintf("reading upload: %s", err),
		)
	}
	tmp.Close()

	// Create a job and start transcoding in the background.
	job := newUploadJob(name, category, file.Filename)
	jobID := job.ID

	go func() {
		defer os.Remove(tmpName)

		preset, err := h.store.SaveFileWithProgress(category, name, tmpName,
			func(percent float64) {
				step := "Transcoding"
				if percent < 0 {
					step = "Transcoding (indeterminate)"
					percent = 0
				}
				updateUploadJob(jobID, percent, step)
			},
		)
		if err != nil {
			h.log.Error("upload: transcode failed", "job", jobID, "name", name, "err", err)
			failUploadJob(jobID, err.Error())
			return
		}

		completeUploadJob(jobID, preset)
		h.log.Info("upload: done", "job", jobID, "name", name, "category", category)
	}()

	// Clean up old completed jobs (keep for 10 minutes so clients can read
	// the final status).
	cleanupOldUploadJobs(10 * time.Minute)

	return c.JSON(http.StatusAccepted, map[string]any{
		"job_id":   jobID,
		"status":   JobTranscoding,
		"name":     name,
		"category": category,
		"filename": file.Filename,
	})
}

// UploadJobStatus handles GET /api/library/upload/jobs/:id — polls the
// progress of an async upload/transcode job.
func (h *Handlers) UploadJobStatus(c echo.Context) error {
	job := getUploadJob(c.Param("id"))
	if job == nil {
		return echo.NewHTTPError(http.StatusNotFound, "job not found")
	}
	return c.JSON(http.StatusOK, job)
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

// PreviewPreset handles GET /api/library/:category/:name/preview — streams WAV
// for audio presets, or redirects to the stream URL for stream presets.
func (h *Handlers) PreviewPreset(c echo.Context) error {
	preset, err := h.store.Get(c.Param("category"), c.Param("name"))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}

	// Stream presets: redirect to the live stream URL so the browser can
	// play it directly (many icecast/shoutcast streams work in <audio>).
	if preset.IsStream() {
		return c.Redirect(http.StatusFound, preset.URL)
	}

	// Audio presets: convert raw → WAV on the fly for browser preview.
	wav, err := rawToWAV(preset.RawPath, h.tmpDir)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer os.Remove(wav)

	return c.File(wav)
}

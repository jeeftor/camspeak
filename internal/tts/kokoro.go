// Package tts provides a client for OpenAI-compatible TTS endpoints (e.g. Kokoro via Lemonade).
package tts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	clog "github.com/charmbracelet/log"
	"github.com/jeeftor/camspeak/internal/logging"
)

var log = logging.New("tts", clog.InfoLevel)

// SetLogLevel updates the TTS client logger level.
func SetLogLevel(level clog.Level) {
	logging.SetLevel(log, level)
}

// Client calls an OpenAI-compatible /v1/audio/speech endpoint.
type Client struct {
	URL    string
	Model  string
	client *http.Client
}

// NewClient creates a TTS client.
func NewClient(url, model string) *Client {
	return &Client{
		URL:   url,
		Model: model,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type speechRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice"`
	ResponseFormat string  `json:"response_format"`
	Speed          float64 `json:"speed,omitempty"`
}

// Speak calls the TTS endpoint and returns WAV audio bytes.
func (c *Client) Speak(text, voice string) ([]byte, error) {
	if voice == "" {
		voice = "af_sky"
	}

	log.Debug("TTS request", "url", c.URL, "model", c.Model, "voice", voice, "text_len", len(text))

	payload, err := json.Marshal(speechRequest{
		Model:          c.Model,
		Input:          text,
		Voice:          voice,
		ResponseFormat: "wav",
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling TTS request: %w", err)
	}

	start := time.Now()
	resp, err := c.client.Post(c.URL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("TTS request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))

		return nil, fmt.Errorf("TTS returned HTTP %d: %s", resp.StatusCode, body)
	}

	wav, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading TTS response: %w", err)
	}

	log.Debug("TTS response", "bytes", len(wav), "elapsed", time.Since(start))

	return wav, nil
}

// Voices fetches available voices from the TTS server (Kokoro-specific endpoint).
// Returns a best-effort list; errors are non-fatal.
func (c *Client) Voices() []string {
	// Kokoro via Lemonade doesn't have a standard voices endpoint yet.
	// Return a curated list of known Kokoro voices.
	return []string{
		"af_sky", "af_bella", "af_nicole", "af_sarah",
		"am_adam", "am_michael",
		"bf_emma", "bf_isabella",
		"bm_george", "bm_lewis",
	}
}

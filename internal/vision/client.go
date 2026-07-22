// Package vision provides an OpenAI-compatible vision LLM client for image description.
package vision

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	clog "github.com/charmbracelet/log"
	"github.com/jeeftor/camspeak/internal/logging"
)

var log = logging.New("vision", clog.InfoLevel)

// SetLogLevel updates the vision client logger level.
func SetLogLevel(level clog.Level) {
	log.SetLevel(level)
	log.SetReportCaller(level == clog.DebugLevel)
}

// Client calls an OpenAI-compatible /v1/chat/completions endpoint with image input.
type Client struct {
	url    string
	model  string
	apiKey string
	client *http.Client
}

// NewClient creates a vision client.
func NewClient(url, model, apiKey string) *Client {
	return &Client{
		url:    url,
		model:  model,
		apiKey: apiKey,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

// Describe sends an image to the vision model and returns a text description.
// imageBytes is the raw image data (JPEG/PNG), mimeType is "image/jpeg" etc.
func (c *Client) Describe(imageBytes []byte, mimeType, prompt string) (string, error) {
	if c.url == "" {
		return "", fmt.Errorf("vision URL not configured")
	}
	if c.model == "" {
		return "", fmt.Errorf("vision model not configured")
	}

	b64 := base64.StdEncoding.EncodeToString(imageBytes)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, b64)

	if prompt == "" {
		prompt = "Describe what you see in one or two sentences. Be concise and factual."
	}

	log.Debug("vision request",
		"url", c.url,
		"model", c.model,
		"image_bytes", len(imageBytes),
		"prompt_len", len(prompt),
	)

	body := fmt.Sprintf(`{
		"model": %q,
		"messages": [{
			"role": "user",
			"content": [
				{"type": "text", "text": %q},
				{"type": "image_url", "image_url": {"url": %q}}
			]
		}],
		"max_tokens": 150,
		"temperature": 0.3
	}`, c.model, prompt, dataURL)

	req, err := http.NewRequest(http.MethodPost, c.url, bytes.NewBufferString(body))
	if err != nil {
		return "", fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	start := time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("vision request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("vision API returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parsing vision response: %w", err)
	}

	if len(result.Choices) == 0 || result.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("vision API returned empty response: %s", string(respBody))
	}

	log.Debug("vision response", "text_len", len(result.Choices[0].Message.Content), "elapsed", time.Since(start))

	return result.Choices[0].Message.Content, nil
}

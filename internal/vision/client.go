// Package vision provides an OpenAI-compatible vision LLM client for image description.
package vision

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	clog "github.com/charmbracelet/log"
	"github.com/jeeftor/camspeak/internal/logging"
)

var log = logging.New("vision", clog.InfoLevel)

// SetLogLevel updates the vision client logger level.
func SetLogLevel(level clog.Level) {
	logging.SetLevel(log, level)
}

// Client calls an OpenAI-compatible /v1/chat/completions endpoint with image input.
type Client struct {
	url    string
	model  string
	apiKey string
	client *http.Client
}

// URL returns the base endpoint URL of the client.
func (c *Client) URL() string { return c.url }

// APIKey returns the API key of the client.
func (c *Client) APIKey() string { return c.apiKey }

// normalizeURL ensures the URL ends with /v1/chat/completions so callers
// can provide either a bare base URL ("http://host:port") or the full path.
func normalizeURL(u string) string {
	u = strings.TrimRight(u, "/")
	if strings.HasSuffix(u, "/v1/chat/completions") {
		return u
	}
	if strings.HasSuffix(u, "/v1") {
		return u + "/chat/completions"
	}
	return u + "/v1/chat/completions"
}

// NewClient creates a vision client.
// url may be a bare base URL ("http://host:port"), end in "/v1", or be the
// full "/v1/chat/completions" path — all are accepted and normalized.
func NewClient(url, model, apiKey string) *Client {
	return &Client{
		url:    normalizeURL(url),
		model:  model,
		apiKey: apiKey,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

// Describe sends an image to the vision model and returns a text description.
// imageBytes is the raw image data (JPEG/PNG), mimeType is "image/jpeg" etc.
func (c *Client) Describe(imageBytes []byte, mimeType, prompt string) (string, error) {
	return c.DescribeWithModel(imageBytes, mimeType, prompt, "")
}

// DescribeWithModel is like Describe but uses modelOverride instead of the configured model.
// Pass an empty string to use the configured model.
func (c *Client) DescribeWithModel(
	imageBytes []byte,
	mimeType, prompt, modelOverride string,
) (string, error) {
	if c.url == "" {
		return "", fmt.Errorf("vision URL not configured")
	}
	model := modelOverride
	if model == "" {
		model = c.model
	}
	if model == "" {
		return "", fmt.Errorf("vision model not configured")
	}

	b64 := base64.StdEncoding.EncodeToString(imageBytes)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, b64)

	if prompt == "" {
		prompt = "Describe what you see in one or two sentences. Be concise and factual."
	}

	log.Debug("vision request",
		"url", c.url,
		"model", model,
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
	}`, model, prompt, dataURL)

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

	log.Debug(
		"vision response",
		"text_len",
		len(result.Choices[0].Message.Content),
		"elapsed",
		time.Since(start),
	)

	return result.Choices[0].Message.Content, nil
}

// DescribeTiming holds per-phase latency from a streaming vision request.
// TtfsMs approximates model-load + prompt-prefill time (time until first token).
// GenMs approximates token-generation time (Total - Ttfs).
type DescribeTiming struct {
	TtfsMs  int64 // ms until first generated token (load + prefill)
	GenMs   int64 // ms for token generation after first token
	TotalMs int64 // total wall-clock ms
}

// DescribeWithModelTimed is like DescribeWithModel but uses streaming to measure
// per-phase latency: time-to-first-token (load + prefill) and generation time.
func (c *Client) DescribeWithModelTimed(
	imageBytes []byte,
	mimeType, prompt, modelOverride string,
) (string, DescribeTiming, error) {
	if c.url == "" {
		return "", DescribeTiming{}, fmt.Errorf("vision URL not configured")
	}
	model := modelOverride
	if model == "" {
		model = c.model
	}
	if model == "" {
		return "", DescribeTiming{}, fmt.Errorf("vision model not configured")
	}

	b64 := base64.StdEncoding.EncodeToString(imageBytes)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, b64)
	if prompt == "" {
		prompt = "Describe what you see in one or two sentences. Be concise and factual."
	}

	body := fmt.Sprintf(`{
		"model": %q,
		"stream": true,
		"messages": [{
			"role": "user",
			"content": [
				{"type": "text", "text": %q},
				{"type": "image_url", "image_url": {"url": %q}}
			]
		}],
		"max_tokens": 150,
		"temperature": 0.3
	}`, model, prompt, dataURL)

	req, err := http.NewRequest(http.MethodPost, c.url, bytes.NewBufferString(body))
	if err != nil {
		return "", DescribeTiming{}, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	start := time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		return "", DescribeTiming{}, fmt.Errorf("vision request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", DescribeTiming{}, fmt.Errorf(
			"vision API returned HTTP %d: %s",
			resp.StatusCode,
			string(body),
		)
	}

	// Read SSE stream; record time to first content token.
	var sb strings.Builder
	var ttfs time.Duration
	gotFirst := false

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}
		for _, ch := range chunk.Choices {
			if ch.Delta.Content != "" {
				if !gotFirst {
					ttfs = time.Since(start)
					gotFirst = true
				}
				sb.WriteString(ch.Delta.Content)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return "", DescribeTiming{}, fmt.Errorf("reading stream: %w", err)
	}

	total := time.Since(start)
	if !gotFirst {
		ttfs = total
	}
	timing := DescribeTiming{
		TtfsMs:  ttfs.Milliseconds(),
		GenMs:   (total - ttfs).Milliseconds(),
		TotalMs: total.Milliseconds(),
	}

	text := strings.TrimSpace(sb.String())
	if text == "" {
		return "", timing, fmt.Errorf("vision API returned empty response")
	}

	log.Debug("vision-timed response",
		"model", model,
		"ttfs_ms", timing.TtfsMs,
		"gen_ms", timing.GenMs,
		"total_ms", timing.TotalMs,
	)
	return text, timing, nil
}

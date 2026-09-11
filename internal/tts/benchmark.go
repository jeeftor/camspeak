package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// BenchmarkResult measures response delivery, not camera or audible playback.
type BenchmarkResult struct {
	Mode        string  `json:"mode"`
	FirstByteMs int64   `json:"first_byte_ms"`
	TotalMs     int64   `json:"total_ms"`
	Bytes       int     `json:"bytes"`
	Duration    float64 `json:"duration"`
	Audio       string  `json:"audio"`
}

// Benchmark compares transport behavior using explicit PCM format assumptions.
// It never sends audio to a camera or retries an uncertain request.
func (c *Client) Benchmark(
	ctx context.Context,
	text, voice, mode string,
	rate, channels int,
) (BenchmarkResult, error) {
	result := BenchmarkResult{Mode: mode}
	if mode != "buffered" && mode != "streaming" {
		return result, fmt.Errorf("mode must be buffered or streaming")
	}
	if rate < 8000 || rate > 96000 || channels < 1 || channels > 2 {
		return result, fmt.Errorf("invalid PCM sample rate or channels")
	}
	body := map[string]any{"model": c.Model, "input": text, "voice": voice, "response_format": "wav"}
	if mode == "streaming" {
		body["response_format"] = "pcm"
		body["stream_format"] = "audio"
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return result, fmt.Errorf("encoding benchmark: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(payload))
	if err != nil {
		return result, fmt.Errorf("invalid TTS endpoint")
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	start := time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		return result, fmt.Errorf("TTS benchmark request failed (connection, timeout, or cancellation)")
	}
	defer resp.Body.Close()
	benchmarkLogger().Info("benchmark response", "mode", mode, "model", c.Model,
		"requested_format", body["response_format"], "status", resp.StatusCode,
		"content_type", resp.Header.Get("Content-Type"), "content_length", resp.ContentLength,
		"transfer_encoding", resp.TransferEncoding, "headers_ms", time.Since(start).Milliseconds(),
		"pcm_rate", rate, "pcm_channels", channels)
	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf(
			"TTS returned HTTP %d; this model/server may not support %s",
			resp.StatusCode,
			mode,
		)
	}
	if ct := resp.Header.Get("Content-Type"); strings.Contains(ct, "json") ||
		strings.Contains(ct, "text/") {
		return result, fmt.Errorf("TTS returned text instead of audio")
	}
	const maxBytes = 8 << 20
	reader := io.LimitReader(resp.Body, maxBytes+1)
	first := make([]byte, 1)
	if _, err := io.ReadFull(reader, first); err != nil {
		return result, fmt.Errorf("TTS returned no audio")
	}
	result.FirstByteMs = time.Since(start).Milliseconds()
	rest, err := io.ReadAll(reader)
	if err != nil {
		return result, fmt.Errorf("TTS audio read failed or was canceled")
	}
	result.TotalMs = time.Since(start).Milliseconds()
	data := first
	data = append(data, rest...)
	if len(data) > maxBytes {
		return result, fmt.Errorf("test audio exceeded 8 MiB; use shorter text")
	}
	result.Bytes = len(data)
	benchmarkLogger().Info("benchmark audio received", "mode", mode, "model", c.Model,
		"bytes", result.Bytes, "first_byte_ms", result.FirstByteMs, "total_ms", result.TotalMs,
		"wav_header", bytes.HasPrefix(data, []byte("RIFF")))
	if mode == "streaming" {
		if len(data) < channels*2 || len(data)%(channels*2) != 0 ||
			bytes.HasPrefix(data, []byte("RIFF")) {
			return result, fmt.Errorf("response is not the requested raw 16-bit PCM")
		}
		// Explicit format is required because raw PCM has no self-describing header.
		result.Duration = float64(len(data)) / float64(rate*channels*2)
		data = pcmWAV(data, rate, channels)
	} else if len(data) < 44 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return result, fmt.Errorf("buffered response is not WAV audio")
	}
	result.Audio = "data:audio/wav;base64," + base64.StdEncoding.EncodeToString(data)
	return result, nil
}

func pcmWAV(data []byte, rate, channels int) []byte {
	header := make([]byte, 44)
	copy(header, "RIFF")
	binary.LittleEndian.PutUint32(header[4:], uint32(36+len(data)))
	copy(header[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(header[16:], 16)
	binary.LittleEndian.PutUint16(header[20:], 1)
	binary.LittleEndian.PutUint16(header[22:], uint16(channels))
	binary.LittleEndian.PutUint32(header[24:], uint32(rate))
	binary.LittleEndian.PutUint32(header[28:], uint32(rate*channels*2))
	binary.LittleEndian.PutUint16(header[32:], uint16(channels*2))
	binary.LittleEndian.PutUint16(header[34:], 16)
	copy(header[36:], "data")
	binary.LittleEndian.PutUint32(header[40:], uint32(len(data)))
	return append(header, data...)
}

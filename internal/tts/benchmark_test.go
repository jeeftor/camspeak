package tts

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBenchmarkStreamingMeasuresDeliveryAndWrapsPCM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["stream_format"] != "audio" || body["response_format"] != "pcm" || body["input"] != "test" || r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("wrong streaming request")
		}
		w.Header().Set("Content-Type", "audio/pcm")
		_, _ = w.Write([]byte{0, 0})
		w.(http.Flusher).Flush()
		time.Sleep(50 * time.Millisecond)
		_, _ = w.Write([]byte{1, 0})
	}))
	defer server.Close()
	result, err := NewClient(server.URL, "kokoro", "secret").Benchmark(context.Background(), "test", "af_sky", "streaming", 24000, 1)
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalMs-result.FirstByteMs < 30 || result.Bytes != 4 {
		t.Fatalf("incorrect timing/size: %+v", result)
	}
	wav, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(result.Audio, "data:audio/wav;base64,"))
	if err != nil || len(wav) != 48 || string(wav[:4]) != "RIFF" {
		t.Fatal("invalid PCM preview wrapper")
	}
}

func TestBenchmarkBufferedAndErrors(t *testing.T) {
	for _, mode := range []string{"buffered", "streaming"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				if mode == "buffered" {
					if _, ok := body["stream_format"]; ok {
						t.Error("buffered request enabled streaming")
					}
					w.Header().Set("Content-Type", "audio/wav")
					_, _ = w.Write(pcmWAV([]byte{0, 0}, 24000, 1))
				} else {
					w.Header().Set("Content-Type", "text/html")
					_, _ = w.Write([]byte("secret error page"))
				}
			}))
			defer server.Close()
			_, err := NewClient(server.URL, "kokoro").Benchmark(context.Background(), "test", "voice", mode, 24000, 1)
			if (err != nil) != (mode == "streaming") {
				t.Fatalf("unexpected error: %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("response leaked")
			}
		})
	}
}

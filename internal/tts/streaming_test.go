package tts

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenPCMRequestAndFormat(t *testing.T) {
	for _, contentType := range []string{"application/octet-stream; charset=binary", "audio/pcm", "audio/mpeg", "text/html"} {
		t.Run(contentType, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body["response_format"] != "pcm" || body["stream_format"] != "audio" {
					t.Errorf("wrong request: %v", body)
				}
				w.Header().Set("Content-Type", contentType)
				_, _ = w.Write([]byte{0, 1, 0, 1, 0, 1})
			}))
			defer server.Close()
			reader, err := NewClient(server.URL, "test").OpenPCM(context.Background(), "hello", "voice")
			valid := contentType == "audio/pcm" ||
				contentType == "application/octet-stream; charset=binary"
			if valid {
				if err != nil {
					t.Fatal(err)
				}
				defer reader.Close()
				data, err := io.ReadAll(reader)
				if err != nil || len(data) != 6 {
					t.Fatalf("lost PCM prefix: %v %v", data, err)
				}
			} else if err == nil {
				reader.Close()
				t.Fatal("non-PCM accepted")
			}
		})
	}
}

func TestStreamingRejectionDiagnostics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("RIFFprivate-audio-never-log-this"))
	}))
	defer server.Close()
	_, err := NewClient(
		server.URL,
		"kokoro-v1",
	).OpenPCM(context.Background(), "private speech", "voice")
	if err == nil {
		t.Fatal("WAV accepted as raw PCM")
	}
	for _, want := range []string{"audio/wav", "WAV/RIFF", "kokoro-v1", "No audio was sent"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q in %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "private") {
		t.Fatal("audio or speech leaked")
	}
}

package tts

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type zeroAudioReader struct{}

func (zeroAudioReader) Read(p []byte) (int, error) { clear(p); return len(p), nil }

func TestBufferedSpeechResponseIsBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = io.CopyN(w, zeroAudioReader{}, (64<<20)+1)
	}))
	defer server.Close()
	data, err := NewClient(server.URL, "test").SpeakContext(context.Background(), "test", "test")
	if err == nil || !strings.Contains(err.Error(), "64 MiB") || data != nil {
		t.Fatalf("oversize response was not rejected: bytes=%d error=%v", len(data), err)
	}
}

package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/config"
)

type testSpeechStreamer struct {
	operationSpeaker
	first chan struct{}
	once  sync.Once
}

func (s *testSpeechStreamer) StreamWithGainContext(
	_ context.Context,
	r io.Reader,
	gain *cameras.GainController,
) error {
	buf := make([]byte, 160)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			gain.RecordLevel(0.2)
			s.once.Do(func() { close(s.first) })
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func TestStreamingSpeechDeliversBeforeGenerationCompletes(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg required")
	}
	speaker := &testSpeechStreamer{first: make(chan struct{})}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(bytes.Repeat([]byte{0, 32}, 24000))
		w.(http.Flusher).Flush()
		select {
		case <-speaker.first:
			_, _ = w.Write(bytes.Repeat([]byte{0, 32}, 2400))
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	op := beginOperation(ctx, "streaming-test", speaker, "speak", "hello")
	defer op.finish()
	cfg := config.Config{
		TTS: config.TTSConfig{URL: server.URL, Streaming: true, PCMSampleRate: 24000, PCMChannels: 1},
	}
	timings := NewStepTimings(4)
	h := &Handlers{}
	if err := h.streamSpeech(op, cfg, "hello", "voice", cameras.NewGainController(1), timings, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := timings.steps["send_open_ms"]; !ok {
		t.Fatal("missing first audio timing")
	}
	if canStreamSpeech(op, config.Config{}) {
		t.Fatal("streaming must default off")
	}
	op.cam = &operationSpeaker{}
	if canStreamSpeech(op, cfg) {
		t.Fatal("unsupported camera must remain buffered")
	}
}

func TestStreamingSpeechCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	op := beginOperation(ctx, "streaming-cancel-test", &testSpeechStreamer{}, "speak", "hello")
	defer op.finish()
	h := &Handlers{}
	err := h.streamSpeech(
		op,
		config.Config{TTS: config.TTSConfig{URL: server.URL}},
		"hello",
		"",
		cameras.NewGainController(1),
		NewStepTimings(4),
		nil,
	)
	if err == nil {
		t.Fatal("canceled speech succeeded")
	}
}

package api

import (
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"sync/atomic"
	"testing"

	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/tts"
)

func TestBroadcastPreparesSpeechOnce(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg required")
	}
	h, _, _ := setupTestHandlers(t)
	var syntheses atomic.Int32
	wav := make([]byte, 44+1600)
	copy(wav[0:], "RIFF")
	binary.LittleEndian.PutUint32(wav[4:], uint32(len(wav)-8))
	copy(wav[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:], 16)
	binary.LittleEndian.PutUint16(wav[20:], 1)
	binary.LittleEndian.PutUint16(wav[22:], 1)
	binary.LittleEndian.PutUint32(wav[24:], 8000)
	binary.LittleEndian.PutUint32(wav[28:], 16000)
	binary.LittleEndian.PutUint16(wav[32:], 2)
	binary.LittleEndian.PutUint16(wav[34:], 16)
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[40:], 1600)
	speech := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		syntheses.Add(1)
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write(wav)
	}))
	defer speech.Close()
	router := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer router.Close()
	h.tts = tts.NewClient(speech.URL, "test")
	if err := h.reg.SetRouting(router.URL, "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"broadcast-a", "broadcast-b"} {
		cfg := config.CameraConfig{
			Type:    "go2rtc",
			Stream:  name,
			IP:      "127.0.0.1",
			Enabled: true,
			Gain:    1,
		}
		if err := h.reg.EnableCamera(name, cfg); err != nil {
			t.Fatal(err)
		}
	}
	result := h.BroadcastToCamerasContext(
		context.Background(),
		[]string{"broadcast-a", "broadcast-b"},
		"hello",
		"",
		"voice",
		0,
	)
	if len(result.Errors) != 0 || len(result.Succeeded) != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if got := syntheses.Load(); got != 1 {
		t.Fatalf("synthesized %d times, want once", got)
	}
}

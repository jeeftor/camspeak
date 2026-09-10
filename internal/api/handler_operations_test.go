package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jeeftor/camspeak/internal/config"
)

func TestCameraSummaryIncludesMutedGainAndCapabilities(t *testing.T) {
	h, e, _ := setupTestHandlers(t)
	h.cfg.Cameras["front"] = config.CameraConfig{Type: "hikvision", Enabled: true, Gain: 0}
	res := doJSON(e, http.MethodGet, "/api/cameras", "")
	var summaries []struct {
		Gain         *float64        `json:"gain"`
		Capabilities map[string]bool `json:"capabilities"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &summaries); err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].Gain == nil || *summaries[0].Gain != 0 {
		t.Fatalf("muted gain missing from dashboard response: %s", res.Body.String())
	}
	if !summaries[0].Capabilities["live_stream"] || !summaries[0].Capabilities["speak"] {
		t.Fatalf("missing camera capabilities: %s", res.Body.String())
	}
}

func TestStopCancelsURLDownload(t *testing.T) {
	h, _, _ := setupTestHandlers(t)
	cam := config.CameraConfig{Type: "hikvision", IP: "127.0.0.1:1", Enabled: true, Gain: 2}
	h.cfg.Cameras["front"] = cam
	h.reg.UpdateConfig("front", cam)
	if err := h.reg.EnableCamera("front", cam); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer download.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- h.doPlayURLContext(ctx, h.log, "front", download.URL, -1) }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("download did not start")
	}
	stopOperation("front")
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled download reported successful playback")
		}
	case <-time.After(time.Second):
		t.Fatal("Stop did not cancel the active download")
	}
}

func TestPlaybackGainRequestDistinguishesOmissionAndMute(t *testing.T) {
	for _, tc := range []struct {
		body string
		want float64
	}{
		{`{}`, -1}, {`{"gain":0}`, 0}, {`{"gain":2.5}`, 2.5},
	} {
		var req speakReq
		if err := json.Unmarshal([]byte(tc.body), &req); err != nil {
			t.Fatal(err)
		}
		if got := requestGain(req.Gain); got != tc.want {
			t.Fatalf("%s gain = %v, want %v", tc.body, got, tc.want)
		}
	}
}

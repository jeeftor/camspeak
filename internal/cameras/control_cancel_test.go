package cameras

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHikvisionStopCancelsPendingOpen(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	cam := NewHikvisionClient(strings.TrimPrefix(server.URL, "http://"), "", "", 1, "test")
	done := make(chan error, 1)
	go func() { _, err := cam.openChannel(); done <- err }()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("open failed before reaching server: %v", err)
	case <-time.After(time.Second):
		t.Fatal("open did not reach server")
	}
	if err := cam.Stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Stop did not cancel channel setup")
	}
}

func TestCanceledSendCannotPreemptCurrentPlayback(t *testing.T) {
	hikvision := NewHikvisionClient("127.0.0.1", "", "", 1, "canceled")
	go2rtc := NewGo2rtcClient("http://127.0.0.1:1", "test", "127.0.0.1", "", "canceled")
	onvif := NewOnvifClient("rtsp://127.0.0.1/test", "127.0.0.1", "canceled")
	tests := []struct {
		name     string
		mu       *sync.Mutex
		activeMu *sync.Mutex
		stopped  *bool
		send     func(context.Context, string, *GainController) (SendTiming, error)
	}{
		{"hikvision", &hikvision.mu, &hikvision.activeMu, &hikvision.stopped, hikvision.SendRawContext},
		{"go2rtc", &go2rtc.mu, &go2rtc.activeMu, &go2rtc.stopped, go2rtc.SendRawContext},
		{"onvif", &onvif.mu, &onvif.activeMu, &onvif.stopped, onvif.SendRawContext},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			tc.mu.Lock()
			defer tc.mu.Unlock()
			done := make(chan error, 1)
			go func() { _, err := tc.send(ctx, "unused", nil); done <- err }()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("got %v, want canceled", err)
				}
			case <-time.After(time.Second):
				t.Fatal("canceled send waited for current playback")
			}
			tc.activeMu.Lock()
			stopped := *tc.stopped
			tc.activeMu.Unlock()
			if stopped {
				t.Fatal("canceled send stopped current playback")
			}
		})
	}
}

func TestQueuedSpeakerLockCanBeCanceled(t *testing.T) {
	var mu sync.Mutex
	mu.Lock()
	defer mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- lockSpeaker(ctx, &mu) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued request did not cancel")
	}
}

package api

import (
	"net/http"
	"testing"
	"time"
)

func TestStopWaitsForPlaybackPublicationBoundary(t *testing.T) {
	h, e, _ := setupTestHandlers(t)
	e.POST("/api/stop", h.Stop)
	h.configEditMu.Lock()
	locked := true
	defer func() {
		if locked {
			h.configEditMu.Unlock()
		}
	}()
	done := make(chan int, 1)
	go func() { done <- doJSON(e, http.MethodPost, "/api/stop", `{}`).Code }()
	select {
	case status := <-done:
		t.Fatalf("Stop crossed the publication boundary: HTTP %d", status)
	case <-time.After(30 * time.Millisecond):
	}
	h.configEditMu.Unlock()
	locked = false
	select {
	case status := <-done:
		if status != http.StatusOK {
			t.Fatalf("Stop failed: HTTP %d", status)
		}
	case <-time.After(time.Second):
		t.Fatal("Stop did not finish after boundary released")
	}
}

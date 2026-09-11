package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jeeftor/camspeak/internal/vision"
)

func TestVisionComparisonDisconnectCancelsInferenceWithoutDoneEvent(t *testing.T) {
	h, e, _ := setupTestHandlers(t)
	entered := make(chan struct{}, 2)
	upstreamCanceled := make(chan struct{}, 2)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			_, _ = w.Write([]byte(`{"data":[{"id":"Qwen3-VL-4B"},{"id":"Qwen3-VL-8B"}]}`))
			return
		}
		entered <- struct{}{}
		// Flush headers so inference is waiting on the streamed response body.
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		upstreamCanceled <- struct{}{}
	}))
	defer upstream.Close()
	h.vision = vision.NewClient(upstream.URL+"/v1/chat/completions", "Qwen3-VL-4B", "")
	e.POST("/api/vision/test-all/stream", h.VisionTestAllStream)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/vision/test-all/stream",
		strings.NewReader(`{"image":"aW1hZ2U=","prompt":"Describe"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { e.ServeHTTP(rec, req); close(done) }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("comparison did not start inference")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("comparison did not stop after disconnect")
	}
	select {
	case <-upstreamCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("upstream inference was not canceled")
	}
	if strings.Contains(rec.Body.String(), `"type":"done"`) ||
		strings.Contains(rec.Body.String(), `"type":"result"`) {
		t.Fatalf("canceled comparison emitted completion/results: %s", rec.Body.String())
	}
	if len(entered) != 0 {
		t.Fatal("comparison started another model after cancellation")
	}
}

package api

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jeeftor/camspeak/internal/config"
)

func TestCaptureSourceMainSubAndExplicitFailure(t *testing.T) {
	var jpegData bytes.Buffer
	if err := jpeg.Encode(&jpegData, image.NewRGBA(image.Rect(0, 0, 4, 4)), nil); err != nil {
		t.Fatal(err)
	}
	paths := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		if strings.Contains(r.URL.Path, "102") {
			http.Error(w, "unsupported", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(jpegData.Bytes())
	}))
	defer server.Close()
	h, _, _ := setupTestHandlers(t)
	cam := config.CameraConfig{
		Type:         "hikvision",
		IP:           strings.TrimPrefix(server.URL, "http://"),
		User:         "test",
		Channel:      1,
		SnapMethod:   "isapi",
		VisionStream: "main",
	}
	_, source, err := h.fetchSnapshotSource(context.Background(), "front", cam, server.URL, "")
	if err != nil || !strings.Contains(source, "main") {
		t.Fatalf("main capture: %s %v", source, err)
	}
	if path := <-paths; !strings.Contains(path, "/101/") {
		t.Fatalf("wrong main path: %s", path)
	}
	cam.VisionStream = "sub"
	if _, _, err := h.fetchSnapshotSource(context.Background(), "front", cam, server.URL, ""); err == nil {
		t.Fatal("explicit failed sub silently fell back")
	}
	if path := <-paths; !strings.Contains(path, "/102/") {
		t.Fatalf("wrong sub path: %s", path)
	}
	cam.SnapMethod = "auto"
	_, source, err = h.fetchSnapshotSource(context.Background(), "front", cam, server.URL, "")
	if err != nil || !strings.Contains(source, "Frigate") {
		t.Fatalf("auto fallback not reported: %s %v", source, err)
	}
}

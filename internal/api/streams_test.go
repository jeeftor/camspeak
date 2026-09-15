package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStreamsSortsNamesCaseInsensitively(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/streams" {
			t.Fatalf("path = %q, want /api/streams", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"zebra_main":{"producers":[]},
			"Alpha_sub":{"producers":[]},
			"backyard_main":{"producers":[]}
		}`))
	}))
	defer upstream.Close()

	h, e, _ := setupTestHandlers(t)
	h.cfg.Go2rtcURL = upstream.URL
	rec := httptest.NewRecorder()
	if err := h.Streams(e.NewContext(httptest.NewRequest(http.MethodGet, "/api/streams", nil), rec)); err != nil {
		t.Fatalf("Streams: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var response struct {
		Streams []StreamInfo `json:"streams"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	got := []string{response.Streams[0].Name, response.Streams[1].Name, response.Streams[2].Name}
	want := []string{"Alpha_sub", "backyard_main", "zebra_main"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("streams = %v, want %v", got, want)
		}
	}
}

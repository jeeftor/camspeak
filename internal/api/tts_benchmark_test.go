package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTTSBenchmarkUsesSavedCredentialsWithoutPlayback(t *testing.T) {
	h, e, database := setupTestHandlers(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer saved-secret" {
			t.Error("missing saved credential")
		}
		w.Header().Set("Content-Type", "audio/pcm")
		_, _ = w.Write([]byte{0, 0, 1, 0})
	}))
	defer server.Close()
	_, err := database.Exec(`INSERT INTO tts_presets(name,endpoint,model,default_voice,api_key) VALUES(?,?,?,?,?)`, "benchmark", server.URL, "kokoro", "af_sky", "saved-secret")
	if err != nil {
		t.Fatal(err)
	}
	e.POST("/api/config/tts/benchmark", h.BenchmarkTTS)
	request := httptest.NewRequest(http.MethodPost, "/api/config/tts/benchmark", strings.NewReader(`{"preset":"benchmark","mode":"streaming","text":"test","sample_rate":24000,"channels":1}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	e.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "data:audio/wav;base64,") || strings.Contains(response.Body.String(), "saved-secret") {
		t.Fatalf("unexpected response: %d %s", response.Code, response.Body.String())
	}
	if count := len(h.reg.Names()); count != 0 {
		t.Fatal("test unexpectedly needed cameras")
	}
}

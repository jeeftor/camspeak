package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jeeftor/camspeak/internal/config"
)

func TestTTSModelCatalogValidation(t *testing.T) {
	for _, tc := range []struct {
		name, body, model string
		ok                bool
	}{
		{"missing model", `{"data":[{"id":"kokoro-v1"}]}`, "kokoro", false},
		{"exact model", `{"data":[{"id":"kokoro-v1"}]}`, "kokoro-v1", true},
		{"catalog only", `{"data":[{"id":"kokoro-v1"}]}`, "", true},
		{"HTML success", `<html>sign in</html>`, "kokoro-v1", false},
		{"empty catalog", `{"data":[]}`, "kokoro-v1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Path != "/v1/models" {
					t.Errorf("unexpected model-loading/audio request: %s %s", r.Method, r.URL.Path)
				}
				_, _ = w.Write([]byte(tc.body))
			}))
			defer upstream.Close()
			h, e, _ := setupTestHandlers(t)
			e.POST("/test", h.TestTTSConfig)
			body, _ := json.Marshal(
				map[string]string{"url": upstream.URL + "/v1/audio/speech", "model": tc.model},
			)
			req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			var got struct {
				OK     bool     `json:"ok"`
				Models []string `json:"models"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.OK != tc.ok {
				t.Fatalf("response = %s", rec.Body.String())
			}
			if tc.name == "missing model" && (len(got.Models) != 1 || got.Models[0] != "kokoro-v1") {
				t.Fatal("missing selection choices")
			}
		})
	}
}

func TestLemonadeDefaultModelID(t *testing.T) {
	for _, p := range config.DefaultTTSPresets {
		if p.Name == "lemonade" && p.Model != "kokoro-v1" {
			t.Fatalf("invalid Lemonade default: %s", p.Model)
		}
	}
}

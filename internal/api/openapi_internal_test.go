package api

import (
	"encoding/json"
	"testing"
)

func TestOpenAPISpecValidAndHasPauseResume(t *testing.T) {
	var v map[string]any
	if err := json.Unmarshal([]byte(openAPISpec), &v); err != nil {
		t.Fatalf("openAPISpec is not valid JSON: %v", err)
	}
	paths, ok := v["paths"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid 'paths'")
	}
	for _, p := range []string{
		"/pause", "/resume", "/stop", "/play-stream", "/playback",
		"/library/upload", "/library/upload/jobs/{id}",
	} {
		if _, ok := paths[p]; !ok {
			t.Errorf("missing path %q in openapi spec", p)
		}
	}
}

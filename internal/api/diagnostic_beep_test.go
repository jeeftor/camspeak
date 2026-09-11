package api

import (
	"context"
	"testing"

	"github.com/jeeftor/camspeak/internal/config"
)

func TestDisabledCameraDiagnosticReservation(t *testing.T) {
	h, _, _ := setupTestHandlers(t)
	h.reg.UpdateConfig(
		"disabled-test",
		config.CameraConfig{Type: "hikvision", IP: "127.0.0.1", Enabled: false},
	)
	cam, err := h.reg.Get("disabled-test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.beginPlaybackOperation(context.Background(), "disabled-test", cam, "speak", "test"); err == nil {
		t.Fatal("normal playback accepted disabled camera")
	}
	op, err := h.beginPlaybackOperation(context.Background(), "disabled-test", cam, "beep", "test")
	if err != nil {
		t.Fatal(err)
	}
	op.finish()
	if _, err := h.reg.GetForPlayback("disabled-test"); err == nil {
		t.Fatal("beep enabled camera")
	}
	for _, name := range h.reg.Names() {
		if name == "disabled-test" {
			t.Fatal("beep enrolled disabled camera")
		}
	}
}

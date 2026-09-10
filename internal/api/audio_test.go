package api

import (
	"testing"

	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/config"
)

// TestResolveGainValue covers the shared numeric gain fallback used by
// effectiveGain and gainForCall.
func TestResolveGainValue(t *testing.T) {
	h, _, _ := setupTestHandlers(t)

	// Register a camera with a known runtime gain via UpdateConfig so the
	// registry's gains map is populated.
	h.cfg.Cameras["front"] = config.CameraConfig{
		Type:    "hikvision",
		IP:      "192.168.1.100",
		Enabled: true,
		Gain:    2.5,
	}
	h.reg.UpdateConfig("front", h.cfg.Cameras["front"])

	tests := []struct {
		name    string
		camera  string
		reqGain float64
		want    float64
	}{
		{"request overrides runtime", "front", 7.0, 7.0},
		{"runtime gain used when no request", "front", -1, 2.5},
		{"default 3.0 for unknown camera", "missing", -1, 3.0},
		{"explicit zero mutes", "front", 0, 0},
		{"request wins even for unknown camera", "missing", 4.0, 4.0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := h.resolveGainValue(tc.camera, tc.reqGain)
			if got != tc.want {
				t.Errorf("resolveGainValue(%q, %v) = %v, want %v",
					tc.camera, tc.reqGain, got, tc.want)
			}
		})
	}
}

// TestGainForCall verifies that gainForCall returns a fresh controller for
// request overrides, the shared registry controller for runtime gain, and a
// default controller for unknown cameras.
func TestGainForCall(t *testing.T) {
	h, _, _ := setupTestHandlers(t)

	h.cfg.Cameras["front"] = config.CameraConfig{
		Type:    "hikvision",
		IP:      "192.168.1.100",
		Enabled: true,
		Gain:    2.5,
	}
	h.reg.UpdateConfig("front", h.cfg.Cameras["front"])
	runtimeGC := h.reg.GetGain("front")

	// Request override → fresh controller, not the registry one.
	gc := h.gainForCall("front", 9.0)
	if gc == runtimeGC {
		t.Error("gainForCall with reqGain returned the shared registry controller")
	}
	if gc.Get() != 9.0 {
		t.Errorf("override controller gain = %v, want 9.0", gc.Get())
	}

	// No request → shared registry controller (so runtime volume changes apply).
	gc = h.gainForCall("front", -1)
	if gc != runtimeGC {
		t.Error("gainForCall without reqGain did not return the registry controller")
	}

	// Unknown camera → default 3.0 controller.
	gc = h.gainForCall("missing", -1)
	if gc == nil {
		t.Fatal("gainForCall returned nil for unknown camera")
	}
	if gc.Get() != 3.0 {
		t.Errorf("default controller gain = %v, want 3.0", gc.Get())
	}

	// Sanity: the default controller is a fresh instance, not shared.
	if gc == cameras.NewGainController(3.0) {
		t.Error("gainForCall returned a shared default controller")
	}
}

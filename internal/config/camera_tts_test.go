package config

import "testing"

func TestCameraTTSModeRoundTrip(t *testing.T) {
	d := newTestDB(t)
	for _, mode := range []string{"streaming", "buffered", ""} {
		if err := SaveCamera(d, "speaker", CameraConfig{Type: "hikvision", Enabled: true, TTSMode: mode}); err != nil {
			t.Fatal(err)
		}
		cfg := Config{Cameras: map[string]CameraConfig{}}
		loadCameras(d, &cfg)
		if cfg.Cameras["speaker"].TTSMode != mode {
			t.Fatalf("mode not persisted: %+v", cfg.Cameras["speaker"])
		}
	}
	if err := SaveCamera(d, "speaker", CameraConfig{TTSMode: "invalid"}); err == nil {
		t.Fatal("invalid mode accepted")
	}
}

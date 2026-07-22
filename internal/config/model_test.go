package config

import (
	"testing"
)

func TestConfigSanitized(t *testing.T) {
	cfg := Config{
		TTS: TTSConfig{
			URL:    "http://tts:8080/v1/audio/speech",
			Model:  "kokoro",
			APIKey: "secret-tts-key",
		},
		Vision: VisionConfig{
			URL:    "http://vision:1234/v1/chat/completions",
			Model:  "qwen",
			APIKey: "secret-vision-key",
		},
		MQTT: MQTTConfig{
			Broker: "tcp://mqtt:1883",
			User:   "mqttuser",
			Pass:   "secret-mqtt-pass",
		},
		Cameras: map[string]CameraConfig{
			"backyard": {
				Type: "hikvision",
				IP:   "192.168.1.181",
				User: "admin",
				Pass: "secret-cam-pass",
			},
		},
	}

	redacted := cfg.Sanitized()

	if redacted.TTS.APIKey != "" {
		t.Errorf("TTS API key not redacted: %q", redacted.TTS.APIKey)
	}
	if redacted.Vision.APIKey != "" {
		t.Errorf("Vision API key not redacted: %q", redacted.Vision.APIKey)
	}
	if redacted.MQTT.Pass != "" {
		t.Errorf("MQTT pass not redacted: %q", redacted.MQTT.Pass)
	}
	if redacted.Cameras["backyard"].Pass != "" {
		t.Errorf("Camera pass not redacted: %q", redacted.Cameras["backyard"].Pass)
	}

	// Ensure non-secret fields are preserved.
	if redacted.TTS.URL != cfg.TTS.URL {
		t.Errorf("TTS URL changed: got %q, want %q", redacted.TTS.URL, cfg.TTS.URL)
	}
	if redacted.Cameras["backyard"].User != "admin" {
		t.Errorf("Camera user changed: got %q, want %q", redacted.Cameras["backyard"].User, "admin")
	}

	// Ensure original config is not mutated.
	if cfg.TTS.APIKey != "secret-tts-key" {
		t.Error("original TTS API key was mutated")
	}
	if cfg.Cameras["backyard"].Pass != "secret-cam-pass" {
		t.Error("original camera pass was mutated")
	}
}

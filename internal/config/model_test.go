package config

import (
	"testing"
)

func TestConfigSanitized(t *testing.T) {
	cfg := Config{
		TTS: TTSConfig{
			URL:    "http://user:pass@tts:8080/v1/audio/speech",
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

	// Ensure URL credentials are stripped but the rest of the URL is preserved.
	if redacted.TTS.URL != "http://tts:8080/v1/audio/speech" {
		t.Errorf("TTS URL not redacted: got %q", redacted.TTS.URL)
	}
	if cfg.TTS.URL != "http://user:pass@tts:8080/v1/audio/speech" {
		t.Error("original TTS URL was mutated")
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

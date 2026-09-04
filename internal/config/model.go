// Package config defines the configuration model for camspeak.
// Configuration is loaded from SQLite preferences with env var overrides.
// YAML files are no longer used.
package config

import (
	clog "github.com/charmbracelet/log"

	"github.com/jeeftor/camspeak/internal/logging"
	"github.com/jeeftor/camspeak/internal/util"
)

// TTSConfig holds connection details for the OpenAI-compatible TTS endpoint.
type TTSConfig struct {
	URL          string `json:"url"`
	Model        string `json:"model"`
	DefaultVoice string `json:"default_voice"`
	APIKey       string `json:"api_key,omitempty"`
}

// Sanitized returns a copy of c with secrets redacted and URL credentials removed.
func (c TTSConfig) Sanitized() TTSConfig {
	out := c
	out.APIKey = ""
	out.URL = util.RedactURLString(out.URL)
	return out
}

// VisionConfig holds connection details for the vision LLM endpoint.
type VisionConfig struct {
	URL    string `json:"url"`
	Model  string `json:"model"`
	APIKey string `json:"api_key,omitempty"`
	Prompt string `json:"prompt"` // global default prompt; empty = hardcoded fallback
}

// Sanitized returns a copy of c with the API key removed and URL credentials stripped.
func (c VisionConfig) Sanitized() VisionConfig {
	out := c
	out.APIKey = ""
	out.URL = util.RedactURLString(out.URL)
	return out
}

// CameraConfig holds connection details for a single camera.
type CameraConfig struct {
	Type           string  `json:"type"` // "hikvision", "reolink", "go2rtc", "onvif"
	IP             string  `json:"ip"`
	User           string  `json:"user"`
	Pass           string  `json:"pass"`
	Channel        int     `json:"channel"`         // Hikvision audio channel, default 1
	Stream         string  `json:"stream"`          // go2rtc stream name (e.g. "garage_2way") or RTSP path for onvif
	Enabled        bool    `json:"enabled"`         // if false, camera is loaded but skipped for speak/broadcast
	AirPlayEnabled bool    `json:"airplay_enabled"` // if false, no shairport-sync receiver for this camera
	AirPlayName    string  `json:"airplay_name"`    // custom AirPlay display name; empty = auto ("Backyard Camera")
	AirPlayModel   string  `json:"airplay_model"`   // custom AirPlay device model/icon; empty = use global AirPlay.Model
	Gain           float64 `json:"gain"`            // digital gain applied to all audio sent to this camera (default 3.0)
	VisionPrompt   string  `json:"vision_prompt"`   // default prompt for vision/describe; empty = generic
	VisionStream   string  `json:"vision_stream"`   // go2rtc stream name for vision snapshots (e.g. "frontyard_sub"); empty = Frigate detect
	VisionWidth    int     `json:"vision_width"`    // max width in px for vision snapshots (0 = no resize); e.g. 1280
	Note           string  `json:"note"`            // limitation/warning shown in UI (e.g. "Limited — see docs")
}

// Sanitized returns a copy of c with the password removed.
func (c CameraConfig) Sanitized() CameraConfig {
	out := c
	out.Pass = ""
	return out
}

// MQTTConfig holds connection details for the MQTT broker.
type MQTTConfig struct {
	Broker string `json:"broker"`
	User   string `json:"user"`
	Pass   string `json:"pass"`
}

// Sanitized returns a copy of c with the password removed.
func (c MQTTConfig) Sanitized() MQTTConfig {
	out := c
	out.Pass = ""
	return out
}

// Rule defines an MQTT-triggered auto-speak rule.
type Rule struct {
	ID      int               `json:"id"`
	Topic   string            `json:"topic"`
	Filter  map[string]string `json:"filter"`
	Cameras []string          `json:"cameras"`
	Preset  string            `json:"preset"`
	Text    string            `json:"text"`
	Voice   string            `json:"voice"`
	Loop    int               `json:"loop"`
	Enabled bool              `json:"enabled"`
}

// TTSPreset is a named TTS endpoint configuration (klipbord-style).
type TTSPreset struct {
	Name         string `json:"name"`
	Endpoint     string `json:"endpoint"`
	Model        string `json:"model"`
	APIKey       string `json:"api_key,omitempty"`
	DefaultVoice string `json:"default_voice"`
	Description  string `json:"description,omitempty"`
	IsActive     bool   `json:"is_active"`
}

// VisionPrompt is a saved named vision prompt for reuse in the vision test playground.
type VisionPrompt struct {
	Name        string `json:"name"`
	Prompt      string `json:"prompt"`
	Description string `json:"description,omitempty"`
}

// Config is the root configuration model.
type Config struct {
	TTS            TTSConfig               `json:"tts"`
	Vision         VisionConfig            `json:"vision"`
	Cameras        map[string]CameraConfig `json:"cameras"`
	MQTT           MQTTConfig              `json:"mqtt"`
	Rules          []Rule                  `json:"rules"`
	Library        string                  `json:"library"`
	Port           int                     `json:"port"`
	FrigateURL     string                  `json:"frigate_url,omitempty"`
	Go2rtcURL      string                  `json:"go2rtc_url,omitempty"`
	AdvertiseIP    string                  `json:"advertise_ip,omitempty"`
	AirPlay        AirPlayConfig           `json:"airplay"`
	PrimeSilenceMs int                     `json:"prime_silence_ms,omitempty"` // ms of silence to prepend before audio to warm camera speaker (default 150, 0=off)
}

// AirPlayConfig controls the RAOP (AirPlay v1) receiver feature.
// When enabled, each camera appears as a separate AirPlay target in the iOS picker.
type AirPlayConfig struct {
	Enabled        bool    `json:"enabled"`          // if true, start AirPlay receivers for all cameras
	BasePort       int     `json:"base_port"`        // starting port for RAOP listeners (default 5000)
	PrimeSilenceMs int     `json:"prime_silence_ms"` // ms of silence to prepend on stream start to warm camera audio engine (default 500)
	Model          string  `json:"model"`            // device model string advertised over mDNS (controls iOS icon)
	Gain           float64 `json:"gain"`             // digital gain applied to AirPlay audio before sending to camera (default 1.0)
}

// Sanitized returns a copy of cfg with secrets (API keys and passwords) redacted.
func (cfg Config) Sanitized() Config {
	out := cfg
	out.TTS = out.TTS.Sanitized()
	out.Vision = out.Vision.Sanitized()
	out.MQTT = out.MQTT.Sanitized()
	if len(cfg.Cameras) > 0 {
		out.Cameras = make(map[string]CameraConfig, len(cfg.Cameras))
		for name, cam := range cfg.Cameras {
			out.Cameras[name] = cam.Sanitized()
		}
	}
	return out
}

var cfgLog = logging.New("config", clog.InfoLevel)

// Defaults.
const (
	defaultPort            = 8585
	defaultLibrary         = "/config/library"
	defaultAirPlayBasePort = 5100
	defaultAirPlayModel    = "RealityDevice14,1"
)

// DefaultTTSPresets are created on first boot if no presets exist.
var DefaultTTSPresets = []TTSPreset{
	{
		Name:         "lemonade",
		Endpoint:     "http://localhost:13305/v1/audio/speech",
		Model:        "kokoro",
		DefaultVoice: "af_sky",
		Description:  "Local Lemonade (GPU)",
		IsActive:     true,
	},
	{
		Name:         "openai",
		Endpoint:     "https://api.openai.com/v1/audio/speech",
		Model:        "tts-1",
		DefaultVoice: "alloy",
		Description:  "OpenAI cloud (requires API key)",
	},
}

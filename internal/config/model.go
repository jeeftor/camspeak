// Package config defines the configuration model for camspeak.
// Configuration is loaded from SQLite preferences with env var overrides.
// YAML files are no longer used.
package config

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	clog "github.com/charmbracelet/log"
	"github.com/joho/godotenv"
)

// TTSConfig holds connection details for the OpenAI-compatible TTS endpoint.
type TTSConfig struct {
	URL          string `json:"url"`
	Model        string `json:"model"`
	DefaultVoice string `json:"default_voice"`
	APIKey       string `json:"api_key,omitempty"`
}

// VisionConfig holds connection details for the vision LLM endpoint.
type VisionConfig struct {
	URL    string `json:"url"`
	Model  string `json:"model"`
	APIKey string `json:"api_key,omitempty"`
	Prompt string `json:"prompt"` // global default prompt; empty = hardcoded fallback
}

// CameraConfig holds connection details for a single camera.
type CameraConfig struct {
	Type         string `json:"type"` // "hikvision", "reolink", "go2rtc", "onvif"
	IP           string `json:"ip"`
	User         string `json:"user"`
	Pass         string `json:"pass"`
	Channel      int    `json:"channel"`       // Hikvision audio channel, default 1
	Stream       string `json:"stream"`        // go2rtc stream name (e.g. "garage_2way") or RTSP path for onvif
	Enabled      bool   `json:"enabled"`       // if false, camera is loaded but skipped for speak/broadcast
	VisionPrompt string `json:"vision_prompt"` // default prompt for vision/describe; empty = generic
}

// MQTTConfig holds connection details for the MQTT broker.
type MQTTConfig struct {
	Broker string `json:"broker"`
	User   string `json:"user"`
	Pass   string `json:"pass"`
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

// Config is the root configuration model.
type Config struct {
	TTS         TTSConfig               `json:"tts"`
	Vision      VisionConfig            `json:"vision"`
	Cameras     map[string]CameraConfig `json:"cameras"`
	MQTT        MQTTConfig              `json:"mqtt"`
	Rules       []Rule                  `json:"rules"`
	Library     string                  `json:"library"`
	Port        int                     `json:"port"`
	FrigateURL  string                  `json:"frigate_url,omitempty"`
	Go2rtcURL   string                  `json:"go2rtc_url,omitempty"`
	AdvertiseIP string                  `json:"advertise_ip,omitempty"`
}

// Defaults.
const (
	defaultPort    = 8585
	defaultLibrary = "/config/library"
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

// Load reads configuration from SQLite + env vars.
// The db must already have the schema initialized.
func Load(db *sql.DB) (*Config, error) {
	// Load .env if present (silently ignore if not found).
	_ = godotenv.Load()

	cfg := &Config{
		Cameras: make(map[string]CameraConfig),
		Port:    defaultPort,
		Library: defaultLibrary,
	}

	// Load preferences from SQLite
	loadPreferences(db, cfg)

	// Load TTS preset (active one)
	loadTTSPreset(db, cfg)

	// Load cameras from SQLite
	loadCameras(db, cfg)

	// Load rules from SQLite
	loadRules(db, cfg)

	// Apply env var overrides (env always wins)
	applyEnvOverrides(cfg)

	// Default channel to 1
	for name, cam := range cfg.Cameras {
		if cam.Channel == 0 {
			cam.Channel = 1
			cfg.Cameras[name] = cam
		}
	}

	return cfg, nil
}

// loadPreferences reads key-value preferences from the preferences table.
func loadPreferences(db *sql.DB, cfg *Config) {
	rows, err := db.Query(`SELECT key, value FROM preferences`)
	if err != nil {
		return
	}
	defer rows.Close()

	prefs := make(map[string]string)
	for rows.Next() {
		var key, val string
		if rows.Scan(&key, &val) == nil {
			prefs[key] = val
		}
	}

	if v, ok := prefs["port"]; ok {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Port = p
		}
	}
	if v, ok := prefs["library"]; ok {
		cfg.Library = v
	}
	if v, ok := prefs["frigate_url"]; ok {
		cfg.FrigateURL = v
	}
	if v, ok := prefs["go2rtc_url"]; ok {
		cfg.Go2rtcURL = v
	}
	if v, ok := prefs["advertise_ip"]; ok {
		cfg.AdvertiseIP = v
	}
	if v, ok := prefs["mqtt_broker"]; ok {
		cfg.MQTT.Broker = v
	}
	if v, ok := prefs["mqtt_user"]; ok {
		cfg.MQTT.User = v
	}
	if v, ok := prefs["mqtt_pass"]; ok {
		cfg.MQTT.Pass = v
	}
	if v, ok := prefs["vision_url"]; ok {
		cfg.Vision.URL = v
	}
	if v, ok := prefs["vision_model"]; ok {
		cfg.Vision.Model = v
	}
	if v, ok := prefs["vision_api_key"]; ok {
		cfg.Vision.APIKey = v
	}
	if v, ok := prefs["vision_prompt"]; ok {
		cfg.Vision.Prompt = v
	}
}

// loadTTSPreset loads the active TTS preset from SQLite.
func loadTTSPreset(db *sql.DB, cfg *Config) {
	var preset TTSPreset
	err := db.QueryRow(
		`SELECT name, endpoint, model, api_key, default_voice, description, is_active
		 FROM tts_presets WHERE is_active = 1 LIMIT 1`,
	).Scan(&preset.Name, &preset.Endpoint, &preset.Model, &preset.APIKey,
		&preset.DefaultVoice, &preset.Description, &preset.IsActive)
	if err == nil {
		cfg.TTS = TTSConfig{
			URL:          preset.Endpoint,
			Model:        preset.Model,
			DefaultVoice: preset.DefaultVoice,
			APIKey:       preset.APIKey,
		}
		return
	}

	// No active preset — seed defaults
	seedDefaultPresets(db)

	// Try again with the first default
	cfg.TTS = TTSConfig{
		URL:          DefaultTTSPresets[0].Endpoint,
		Model:        DefaultTTSPresets[0].Model,
		DefaultVoice: DefaultTTSPresets[0].DefaultVoice,
	}
}

// seedDefaultPresets inserts the default TTS presets if the table is empty.
func seedDefaultPresets(db *sql.DB) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tts_presets`).Scan(&count); err != nil {
		return
	}
	if count > 0 {
		return
	}
	for _, p := range DefaultTTSPresets {
		isActive := 0
		if p.IsActive {
			isActive = 1
		}
		if _, err := db.Exec(
			`INSERT INTO tts_presets (name, endpoint, model, api_key, default_voice, description, is_active)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			p.Name,
			p.Endpoint,
			p.Model,
			p.APIKey,
			p.DefaultVoice,
			p.Description,
			isActive,
		); err != nil {
			clog.Error("config: seeding default TTS preset failed", "name", p.Name, "err", err)
		}
	}
}

// loadCameras loads camera configurations from SQLite.
func loadCameras(db *sql.DB, cfg *Config) {
	rows, err := db.Query(
		`SELECT name, type, ip, user, pass, channel, stream, enabled, vision_prompt FROM cameras`,
	)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var cam CameraConfig
		var name string
		var enabled int
		if err := rows.Scan(&name, &cam.Type, &cam.IP, &cam.User, &cam.Pass, &cam.Channel, &cam.Stream, &enabled, &cam.VisionPrompt); err != nil {
			continue
		}
		cam.Enabled = enabled == 1
		cfg.Cameras[name] = cam
	}
}

// loadRules loads MQTT rules from SQLite.
func loadRules(db *sql.DB, cfg *Config) {
	rows, err := db.Query(
		`SELECT id, topic, filter, cameras, preset, text, voice, enabled FROM rules WHERE enabled = 1`,
	)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var r Rule
		var filterJSON, camerasCSV string
		var enabled int
		if err := rows.Scan(&r.ID, &r.Topic, &filterJSON, &camerasCSV, &r.Preset, &r.Text, &r.Voice, &enabled); err != nil {
			continue
		}
		r.Enabled = enabled == 1
		r.Filter = parseFilterJSON(filterJSON)
		r.Cameras = parseCSV(camerasCSV)
		cfg.Rules = append(cfg.Rules, r)
	}
}

// applyEnvOverrides applies environment variable overrides on top of SQLite config.
// Env vars always win.
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("CAMSPEAK_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Port = p
		}
	}
	if v := os.Getenv("CAMSPEAK_LIBRARY"); v != "" {
		cfg.Library = v
	}
	if v := os.Getenv("CAMSPEAK_FRIGATE_URL"); v != "" {
		cfg.FrigateURL = v
	}
	if v := os.Getenv("CAMSPEAK_GO2RTC_URL"); v != "" {
		cfg.Go2rtcURL = v
	}
	if v := os.Getenv("CAMSPEAK_ADVERTISE_IP"); v != "" {
		cfg.AdvertiseIP = v
	}
	if v := os.Getenv("CAMSPEAK_TTS_URL"); v != "" {
		cfg.TTS.URL = v
	}
	if v := os.Getenv("CAMSPEAK_TTS_MODEL"); v != "" {
		cfg.TTS.Model = v
	}
	if v := os.Getenv("CAMSPEAK_TTS_VOICE"); v != "" {
		cfg.TTS.DefaultVoice = v
	}
	if v := os.Getenv("CAMSPEAK_TTS_API_KEY"); v != "" {
		cfg.TTS.APIKey = v
	}
	if v := os.Getenv("CAMSPEAK_VISION_URL"); v != "" {
		cfg.Vision.URL = v
	}
	if v := os.Getenv("CAMSPEAK_VISION_MODEL"); v != "" {
		cfg.Vision.Model = v
	}
	if v := os.Getenv("CAMSPEAK_VISION_API_KEY"); v != "" {
		cfg.Vision.APIKey = v
	}
	if v := os.Getenv("CAMSPEAK_VISION_PROMPT"); v != "" {
		cfg.Vision.Prompt = v
	}
	if v := os.Getenv("CAMSPEAK_MQTT_BROKER"); v != "" {
		cfg.MQTT.Broker = v
	}
	if v := os.Getenv("CAMSPEAK_MQTT_USER"); v != "" {
		cfg.MQTT.User = v
	}
	if v := os.Getenv("CAMSPEAK_MQTT_PASS"); v != "" {
		cfg.MQTT.Pass = v
	}

	// Per-camera env overrides: CAM_<NAME>_IP, CAM_<NAME>_USER, CAM_<NAME>_PASS
	for name, cam := range cfg.Cameras {
		envName := envCamName(name)
		if v := os.Getenv("CAM_" + envName + "_IP"); v != "" {
			cam.IP = v
		}
		if v := os.Getenv("CAM_" + envName + "_USER"); v != "" {
			cam.User = v
		}
		if v := os.Getenv("CAM_" + envName + "_PASS"); v != "" {
			cam.Pass = v
		}
		cfg.Cameras[name] = cam
	}
}

// envCamName converts a camera name to env var format (uppercase, underscores).
func envCamName(name string) string {
	return strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
}

// SetPreference writes a key-value preference to SQLite.
func SetPreference(db *sql.DB, key, value string) error {
	_, err := db.Exec(
		`INSERT INTO preferences (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	if err != nil {
		return fmt.Errorf("setting preference %s: %w", key, err)
	}
	return nil
}

// SaveCamera inserts or updates a camera in SQLite.
func SaveCamera(db *sql.DB, name string, cam CameraConfig) error {
	enabled := 0
	if cam.Enabled {
		enabled = 1
	}
	_, err := db.Exec(
		`INSERT INTO cameras (name, type, ip, user, pass, channel, stream, enabled, vision_prompt)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
		   type = excluded.type, ip = excluded.ip, user = excluded.user,
		   pass = excluded.pass, channel = excluded.channel, stream = excluded.stream,
		   enabled = excluded.enabled, vision_prompt = excluded.vision_prompt`,
		name, cam.Type, cam.IP, cam.User, cam.Pass, cam.Channel, cam.Stream, enabled, cam.VisionPrompt,
	)
	if err != nil {
		return fmt.Errorf("saving camera %s: %w", name, err)
	}
	return nil
}

// DeleteCamera removes a camera from SQLite.
func DeleteCamera(db *sql.DB, name string) error {
	_, err := db.Exec(`DELETE FROM cameras WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("deleting camera %s: %w", name, err)
	}
	return nil
}

// ListTTSPresets returns all TTS presets from SQLite.
func ListTTSPresets(db *sql.DB) ([]TTSPreset, error) {
	rows, err := db.Query(
		`SELECT name, endpoint, model, api_key, default_voice, description, is_active
		 FROM tts_presets ORDER BY is_active DESC, name`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing TTS presets: %w", err)
	}
	defer rows.Close()

	var presets []TTSPreset
	for rows.Next() {
		var p TTSPreset
		var isActive int
		if err := rows.Scan(&p.Name, &p.Endpoint, &p.Model, &p.APIKey,
			&p.DefaultVoice, &p.Description, &isActive); err != nil {
			continue
		}
		p.IsActive = isActive == 1
		presets = append(presets, p)
	}
	return presets, rows.Err()
}

// SaveTTSPreset inserts or updates a TTS preset.
func SaveTTSPreset(db *sql.DB, p TTSPreset) error {
	isActive := 0
	if p.IsActive {
		isActive = 1
		// Deactivate all other presets
		if _, err := db.Exec(`UPDATE tts_presets SET is_active = 0`); err != nil {
			return fmt.Errorf("deactivating presets: %w", err)
		}
	}
	_, err := db.Exec(
		`INSERT INTO tts_presets (name, endpoint, model, api_key, default_voice, description, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
		   endpoint = excluded.endpoint, model = excluded.model, api_key = excluded.api_key,
		   default_voice = excluded.default_voice, description = excluded.description,
		   is_active = excluded.is_active`,
		p.Name, p.Endpoint, p.Model, p.APIKey, p.DefaultVoice, p.Description, isActive,
	)
	if err != nil {
		return fmt.Errorf("saving TTS preset %s: %w", p.Name, err)
	}
	return nil
}

// SetActiveTTSPreset marks a preset as active and deactivates all others.
func SetActiveTTSPreset(db *sql.DB, name string) error {
	// Verify the preset exists
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tts_presets WHERE name = ?`, name).Scan(&count); err != nil {
		return fmt.Errorf("checking preset: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("TTS preset %q not found", name)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	if _, err := tx.Exec(`UPDATE tts_presets SET is_active = 0`); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("deactivating presets: %w", err)
	}
	if _, err := tx.Exec(`UPDATE tts_presets SET is_active = 1 WHERE name = ?`, name); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("activating preset: %w", err)
	}
	return tx.Commit()
}

// DeleteTTSPreset removes a TTS preset (cannot delete the active one).
func DeleteTTSPreset(db *sql.DB, name string) error {
	var isActive int
	if err := db.QueryRow(`SELECT is_active FROM tts_presets WHERE name = ?`, name).Scan(&isActive); err != nil {
		return fmt.Errorf("checking preset status: %w", err)
	}
	if isActive == 1 {
		return fmt.Errorf("cannot delete the active TTS preset")
	}
	_, err := db.Exec(`DELETE FROM tts_presets WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("deleting TTS preset %s: %w", name, err)
	}
	return nil
}

// parseFilterJSON parses a JSON filter string into a map.
func parseFilterJSON(jsonStr string) map[string]string {
	if jsonStr == "" || jsonStr == "{}" {
		return make(map[string]string)
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		return make(map[string]string)
	}
	return m
}

// parseCSV parses a comma-separated string into a slice.
func parseCSV(csv string) []string {
	if csv == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts
}

package config

import (
	"database/sql"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

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
	if v, ok := prefs["airplay_enabled"]; ok {
		cfg.AirPlay.Enabled = v == "1" || v == "true"
	}
	if v, ok := prefs["airplay_base_port"]; ok {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.AirPlay.BasePort = p
		}
	}
	if v, ok := prefs["airplay_prime_silence_ms"]; ok {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.AirPlay.PrimeSilenceMs = p
		}
	}
	if v, ok := prefs["airplay_model"]; ok {
		cfg.AirPlay.Model = v
	}
	if v, ok := prefs["airplay_gain"]; ok {
		if g, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.AirPlay.Gain = g
		}
	}
	if v, ok := prefs["prime_silence_ms"]; ok {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.PrimeSilenceMs = p
		}
	}
	// Default base port
	if cfg.AirPlay.BasePort == 0 {
		cfg.AirPlay.BasePort = defaultAirPlayBasePort
	}
	// Default prime silence
	if cfg.AirPlay.PrimeSilenceMs == 0 {
		cfg.AirPlay.PrimeSilenceMs = 500
	}
	// Default advertised device model
	if cfg.AirPlay.Model == "" {
		cfg.AirPlay.Model = defaultAirPlayModel
	}
	// Default gain
	if cfg.AirPlay.Gain == 0 {
		cfg.AirPlay.Gain = 1.0
	}
	// Default global prime silence for non-AirPlay paths (TTS, play, stream)
	if cfg.PrimeSilenceMs == 0 {
		cfg.PrimeSilenceMs = 150
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
			log.Error("seeding default TTS preset failed", "name", p.Name, "err", err)
		}
	}
}

// loadCameras loads camera configurations from SQLite.
func loadCameras(db *sql.DB, cfg *Config) {
	rows, err := db.Query(
		`SELECT name, type, ip, user, pass, channel, stream, enabled, vision_prompt,
		        COALESCE(airplay_enabled, 1), COALESCE(airplay_name, ''), COALESCE(airplay_model, ''), COALESCE(gain, 3.0), COALESCE(note, ''),
		        COALESCE(vision_stream, ''), COALESCE(vision_width, 0) FROM cameras`,
	)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var cam CameraConfig
		var name string
		var enabled, airplayEnabled int
		if err := rows.Scan(
			&name, &cam.Type, &cam.IP, &cam.User, &cam.Pass,
			&cam.Channel, &cam.Stream, &enabled, &cam.VisionPrompt, &airplayEnabled, &cam.AirPlayName, &cam.AirPlayModel, &cam.Gain, &cam.Note,
			&cam.VisionStream, &cam.VisionWidth,
		); err != nil {
			continue
		}
		cam.Enabled = enabled == 1
		cam.AirPlayEnabled = airplayEnabled == 1
		// For Reolink cameras with no stream set, default the go2rtc stream
		// name to the camera name so audio is routed through go2rtc.
		// This fixes cameras discovered before the stream field was populated.
		if cam.Type == "reolink" && cam.Stream == "" {
			cam.Stream = name
		}
		cfg.Cameras[name] = cam
	}
}

// loadRules loads MQTT rules from SQLite.
func loadRules(db *sql.DB, cfg *Config) {
	rows, err := db.Query(
		`SELECT id, topic, filter, cameras, preset, text, voice, loop, enabled FROM rules WHERE enabled = 1`,
	)
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var r Rule
		var filterJSON, camerasCSV string
		var enabled, loop int
		if err := rows.Scan(&r.ID, &r.Topic, &filterJSON, &camerasCSV, &r.Preset, &r.Text, &r.Voice, &loop, &enabled); err != nil {
			continue
		}
		r.Enabled = enabled == 1
		r.Loop = loop
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
	if v := os.Getenv("CAMSPEAK_AIRPLAY_ENABLED"); v != "" {
		cfg.AirPlay.Enabled = v == "1" || v == "true" || v == "yes"
	}
	if v := os.Getenv("CAMSPEAK_AIRPLAY_BASE_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.AirPlay.BasePort = p
		}
	}
	if v := os.Getenv("CAMSPEAK_AIRPLAY_PRIME_SILENCE_MS"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.AirPlay.PrimeSilenceMs = p
		}
	}
	if v := os.Getenv("CAMSPEAK_AIRPLAY_MODEL"); v != "" {
		cfg.AirPlay.Model = v
	}
	if v := os.Getenv("CAMSPEAK_AIRPLAY_GAIN"); v != "" {
		if g, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.AirPlay.Gain = g
		}
	}
	if v := os.Getenv("CAMSPEAK_PRIME_SILENCE_MS"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.PrimeSilenceMs = p
		}
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

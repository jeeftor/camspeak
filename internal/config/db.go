package config

import (
	"database/sql"
	"fmt"
)

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

// SetPreferences persists a group of settings atomically.
func SetPreferences(db *sql.DB, preferences map[string]string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("beginning preferences transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	for key, value := range preferences {
		if _, err := tx.Exec(`INSERT INTO preferences (key, value) VALUES (?, ?)
			ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
			return fmt.Errorf("setting preference %s: %w", key, err)
		}
	}
	return tx.Commit()
}

// SaveCamera inserts or updates a camera in SQLite.
func SaveCamera(db *sql.DB, name string, cam CameraConfig) error {
	if cam.TTSMode != "" && cam.TTSMode != "buffered" && cam.TTSMode != "streaming" {
		return fmt.Errorf("invalid camera TTS mode %q", cam.TTSMode)
	}
	enabled := 0
	if cam.Enabled {
		enabled = 1
	}
	airplayEnabled := 1
	if !cam.AirPlayEnabled {
		airplayEnabled = 0
	}
	_, err := db.Exec(
		`INSERT INTO cameras
		   (name, type, ip, user, pass, channel, stream, enabled, vision_prompt,
		    airplay_enabled, airplay_name, airplay_model, gain, note,
		    vision_stream, vision_width, snap_method, sort_order, tts_mode)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
		   type = excluded.type, ip = excluded.ip, user = excluded.user,
		   pass = excluded.pass, channel = excluded.channel, stream = excluded.stream,
		   enabled = excluded.enabled, vision_prompt = excluded.vision_prompt,
		   airplay_enabled = excluded.airplay_enabled, airplay_name = excluded.airplay_name,
		   airplay_model = excluded.airplay_model, gain = excluded.gain, note = excluded.note,
		   vision_stream = excluded.vision_stream, vision_width = excluded.vision_width,
		   snap_method = excluded.snap_method, sort_order = excluded.sort_order, tts_mode = excluded.tts_mode`,
		name,
		cam.Type,
		cam.IP,
		cam.User,
		cam.Pass,
		cam.Channel,
		cam.Stream,
		enabled,
		cam.VisionPrompt,
		airplayEnabled,
		cam.AirPlayName,
		cam.AirPlayModel,
		cam.Gain,
		cam.Note,
		cam.VisionStream,
		cam.VisionWidth,
		cam.SnapMethod,
		cam.SortOrder,
		cam.TTSMode,
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
		`SELECT name, endpoint, model, api_key, default_voice, description, is_active, streaming, pcm_sample_rate, pcm_channels
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
			&p.DefaultVoice, &p.Description, &isActive, &p.Streaming, &p.PCMSampleRate, &p.PCMChannels); err != nil {
			continue
		}
		p.IsActive = isActive == 1
		presets = append(presets, p)
	}
	return presets, rows.Err()
}

// SaveTTSPreset inserts or updates a TTS preset.
func SaveTTSPreset(db *sql.DB, p TTSPreset) error {
	if p.PCMSampleRate == 0 {
		p.PCMSampleRate = 24000
	}
	if p.PCMChannels == 0 {
		p.PCMChannels = 1
	}
	if p.PCMSampleRate < 8000 || p.PCMSampleRate > 96000 || p.PCMChannels < 1 || p.PCMChannels > 2 {
		return fmt.Errorf("invalid PCM sample rate or channels")
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("beginning TTS transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	isActive := 0
	if p.IsActive {
		isActive = 1
		// Deactivate all other presets
		if _, err := tx.Exec(`UPDATE tts_presets SET is_active = 0`); err != nil {
			return fmt.Errorf("deactivating presets: %w", err)
		}
	}
	// Preserve existing API key when the incoming key is empty.
	// This prevents editing a preset from wiping its stored key.
	apiKey := p.APIKey
	if apiKey == "" && !p.ClearAPIKey {
		var existingKey string
		err := tx.QueryRow(
			`SELECT api_key FROM tts_presets WHERE name = ?`, p.Name,
		).Scan(&existingKey)
		if err == nil {
			apiKey = existingKey
		}
	}
	_, err = tx.Exec(
		`INSERT INTO tts_presets (name, endpoint, model, api_key, default_voice, description, is_active, streaming, pcm_sample_rate, pcm_channels)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
		   endpoint = excluded.endpoint, model = excluded.model, api_key = excluded.api_key,
		   default_voice = excluded.default_voice, description = excluded.description,
		   is_active = excluded.is_active, streaming = excluded.streaming,
		   pcm_sample_rate = excluded.pcm_sample_rate, pcm_channels = excluded.pcm_channels`,
		p.Name,
		p.Endpoint,
		p.Model,
		apiKey,
		p.DefaultVoice,
		p.Description,
		isActive,
		p.Streaming,
		p.PCMSampleRate,
		p.PCMChannels,
	)
	if err != nil {
		return fmt.Errorf("saving TTS preset %s: %w", p.Name, err)
	}
	return tx.Commit()
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

// ListVisionPrompts returns all saved vision prompts from SQLite, ordered by name.
func ListVisionPrompts(db *sql.DB) ([]VisionPrompt, error) {
	rows, err := db.Query(
		`SELECT name, prompt, description FROM vision_prompts ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing vision prompts: %w", err)
	}
	defer rows.Close()

	var prompts []VisionPrompt
	for rows.Next() {
		var p VisionPrompt
		if err := rows.Scan(&p.Name, &p.Prompt, &p.Description); err != nil {
			continue
		}
		prompts = append(prompts, p)
	}
	return prompts, rows.Err()
}

// SaveVisionPrompt inserts or updates a vision prompt by name.
func SaveVisionPrompt(db *sql.DB, p VisionPrompt) error {
	if p.Name == "" {
		return fmt.Errorf("vision prompt name is required")
	}
	_, err := db.Exec(
		`INSERT INTO vision_prompts (name, prompt, description)
		 VALUES (?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
		   prompt = excluded.prompt, description = excluded.description`,
		p.Name, p.Prompt, p.Description,
	)
	if err != nil {
		return fmt.Errorf("saving vision prompt %s: %w", p.Name, err)
	}
	return nil
}

// DeleteVisionPrompt removes a vision prompt by name.
func DeleteVisionPrompt(db *sql.DB, name string) error {
	_, err := db.Exec(`DELETE FROM vision_prompts WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("deleting vision prompt %s: %w", name, err)
	}
	return nil
}

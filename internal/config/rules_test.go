package config

import (
	"database/sql"
	"testing"
)

// TestRuleMigrationSourceCamera verifies that the rules table gets the
// source_camera and prompt columns added during migration, and that
// loadRules correctly reads announce rules (source_camera != "").
func TestRuleMigrationSourceCamera(t *testing.T) {
	d := newTestDB(t)

	// Insert an announce rule directly via SQL.
	_, err := d.Exec(`INSERT INTO rules (topic, filter, cameras, preset, text, voice, loop, enabled, source_camera, prompt)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"frigate/events", `{"type":"person"}`, "frontyard,backyard", "", "", "", 0, 1, "doorbell", "Describe who is at the door")
	if err != nil {
		t.Fatalf("inserting announce rule: %v", err)
	}

	// Insert a standard rule (no source_camera).
	_, err = d.Exec(`INSERT INTO rules (topic, filter, cameras, preset, text, voice, loop, enabled, source_camera, prompt)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"frigate/events", `{"type":"car"}`, "frontyard", "alert", "", "af_sky", 0, 1, "", "")
	if err != nil {
		t.Fatalf("inserting standard rule: %v", err)
	}

	// Load config and verify rules.
	cfg, err := Load(d)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Rules) != 2 {
		t.Fatalf("len(rules) = %d, want 2", len(cfg.Rules))
	}

	// Find the announce rule.
	var announce *Rule
	var standard *Rule
	for i := range cfg.Rules {
		if cfg.Rules[i].SourceCamera != "" {
			announce = &cfg.Rules[i]
		} else {
			standard = &cfg.Rules[i]
		}
	}
	if announce == nil {
		t.Fatal("announce rule not found")
	}
	if announce.SourceCamera != "doorbell" {
		t.Errorf("announce.SourceCamera = %q, want %q", announce.SourceCamera, "doorbell")
	}
	if announce.Prompt != "Describe who is at the door" {
		t.Errorf("announce.Prompt = %q, want %q", announce.Prompt, "Describe who is at the door")
	}
	if len(announce.Cameras) != 2 {
		t.Errorf("announce.Cameras len = %d, want 2", len(announce.Cameras))
	}
	if announce.Filter["type"] != "person" {
		t.Errorf("announce.Filter[type] = %q, want %q", announce.Filter["type"], "person")
	}

	if standard == nil {
		t.Fatal("standard rule not found")
	}
	if standard.SourceCamera != "" {
		t.Errorf("standard.SourceCamera = %q, want empty", standard.SourceCamera)
	}
	if standard.Preset != "alert" {
		t.Errorf("standard.Preset = %q, want %q", standard.Preset, "alert")
	}
}

// TestRuleMigrationColumnsExist verifies that the source_camera and prompt
// columns exist after db.Open (which runs migrations).
func TestRuleMigrationColumnsExist(t *testing.T) {
	d := newTestDB(t)

	// Querying these columns should not error — if the migration didn't run,
	// they won't exist and the query will fail.
	var sourceCamera, prompt string
	err := d.QueryRow(`SELECT source_camera, prompt FROM rules LIMIT 1`).Scan(&sourceCamera, &prompt)
	if err != nil && err != sql.ErrNoRows {
		t.Fatalf("querying source_camera/prompt columns: %v", err)
	}
}

// TestLoadRulesDisabledNotLoaded verifies that disabled rules are not loaded.
func TestLoadRulesDisabledNotLoaded(t *testing.T) {
	d := newTestDB(t)

	// Insert an enabled rule.
	_, err := d.Exec(`INSERT INTO rules (topic, cameras, enabled, source_camera, prompt) VALUES (?, ?, ?, ?, ?)`,
		"frigate/events", "front", 1, "doorbell", "test")
	if err != nil {
		t.Fatalf("inserting enabled rule: %v", err)
	}

	// Insert a disabled rule.
	_, err = d.Exec(`INSERT INTO rules (topic, cameras, enabled, source_camera, prompt) VALUES (?, ?, ?, ?, ?)`,
		"frigate/events", "back", 0, "doorbell", "test2")
	if err != nil {
		t.Fatalf("inserting disabled rule: %v", err)
	}

	cfg, err := Load(d)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Rules) != 1 {
		t.Fatalf("len(rules) = %d, want 1 (disabled rules should not load)", len(cfg.Rules))
	}
	if cfg.Rules[0].Cameras[0] != "front" {
		t.Errorf("loaded rule cameras = %v, want [front]", cfg.Rules[0].Cameras)
	}
}

package config

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/jeeftor/camspeak/internal/db"
)

// newTestDB opens a fresh SQLite database in a temporary directory.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestSetPreference(t *testing.T) {
	d := newTestDB(t)
	if err := SetPreference(d, "port", "8585"); err != nil {
		t.Fatalf("SetPreference: %v", err)
	}
	var value string
	if err := d.QueryRow(`SELECT value FROM preferences WHERE key = ?`, "port").Scan(&value); err != nil {
		t.Fatalf("reading preference: %v", err)
	}
	if value != "8585" {
		t.Errorf("preference value = %q, want %q", value, "8585")
	}
}

func TestSetPreferenceUpsert(t *testing.T) {
	d := newTestDB(t)
	if err := SetPreference(d, "port", "8585"); err != nil {
		t.Fatalf("SetPreference first: %v", err)
	}
	if err := SetPreference(d, "port", "9999"); err != nil {
		t.Fatalf("SetPreference second: %v", err)
	}
	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM preferences WHERE key = ?`, "port").Scan(&count); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	if count != 1 {
		t.Errorf("row count = %d, want 1 (upsert should not duplicate)", count)
	}
	var value string
	if err := d.QueryRow(`SELECT value FROM preferences WHERE key = ?`, "port").Scan(&value); err != nil {
		t.Fatalf("reading preference: %v", err)
	}
	if value != "9999" {
		t.Errorf("preference value = %q, want %q (updated)", value, "9999")
	}
}

func TestSaveCamera(t *testing.T) {
	d := newTestDB(t)
	cam := CameraConfig{
		Type:    "hikvision",
		IP:      "192.168.1.100",
		User:    "admin",
		Pass:    "secret",
		Channel: 1,
		Stream:  "main",
		Enabled: true,
		Gain:    3.0,
		Note:    "front door",
	}
	if err := SaveCamera(d, "front", cam); err != nil {
		t.Fatalf("SaveCamera: %v", err)
	}

	var (
		camType string
		ip      string
		user    string
		pass    string
		channel int
		stream  string
		enabled int
		gain    float64
		note    string
	)
	err := d.QueryRow(
		`SELECT type, ip, user, pass, channel, stream, enabled, gain, note FROM cameras WHERE name = ?`,
		"front",
	).Scan(&camType, &ip, &user, &pass, &channel, &stream, &enabled, &gain, &note)
	if err != nil {
		t.Fatalf("reading camera: %v", err)
	}
	if camType != "hikvision" {
		t.Errorf("type = %q, want %q", camType, "hikvision")
	}
	if ip != "192.168.1.100" {
		t.Errorf("ip = %q, want %q", ip, "192.168.1.100")
	}
	if user != "admin" {
		t.Errorf("user = %q, want %q", user, "admin")
	}
	if pass != "secret" {
		t.Errorf("pass = %q, want %q", pass, "secret")
	}
	if channel != 1 {
		t.Errorf("channel = %d, want 1", channel)
	}
	if stream != "main" {
		t.Errorf("stream = %q, want %q", stream, "main")
	}
	if enabled != 1 {
		t.Errorf("enabled = %d, want 1", enabled)
	}
	if gain != 3.0 {
		t.Errorf("gain = %f, want 3.0", gain)
	}
	if note != "front door" {
		t.Errorf("note = %q, want %q", note, "front door")
	}
}

func TestSaveCameraUpsert(t *testing.T) {
	d := newTestDB(t)
	cam := CameraConfig{
		Type: "hikvision",
		IP:   "192.168.1.100",
	}
	if err := SaveCamera(d, "cam1", cam); err != nil {
		t.Fatalf("SaveCamera first: %v", err)
	}
	cam.IP = "192.168.1.200"
	if err := SaveCamera(d, "cam1", cam); err != nil {
		t.Fatalf("SaveCamera second: %v", err)
	}
	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM cameras WHERE name = ?`, "cam1").Scan(&count); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if count != 1 {
		t.Errorf("row count = %d, want 1 (upsert)", count)
	}
	var ip string
	if err := d.QueryRow(`SELECT ip FROM cameras WHERE name = ?`, "cam1").Scan(&ip); err != nil {
		t.Fatalf("reading ip: %v", err)
	}
	if ip != "192.168.1.200" {
		t.Errorf("ip = %q, want %q (updated)", ip, "192.168.1.200")
	}
}

func TestDeleteCamera(t *testing.T) {
	d := newTestDB(t)
	cam := CameraConfig{Type: "hikvision", IP: "10.0.0.1"}
	if err := SaveCamera(d, "todelete", cam); err != nil {
		t.Fatalf("SaveCamera: %v", err)
	}
	if err := DeleteCamera(d, "todelete"); err != nil {
		t.Fatalf("DeleteCamera: %v", err)
	}
	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM cameras WHERE name = ?`, "todelete").Scan(&count); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if count != 0 {
		t.Errorf("row count = %d, want 0 (deleted)", count)
	}
}

func TestSaveCameraWithNote(t *testing.T) {
	d := newTestDB(t)
	cam := CameraConfig{
		Type: "reolink",
		IP:   "192.168.1.50",
		Note: "Limited — see docs",
	}
	if err := SaveCamera(d, "doorbell", cam); err != nil {
		t.Fatalf("SaveCamera: %v", err)
	}
	var note string
	if err := d.QueryRow(`SELECT note FROM cameras WHERE name = ?`, "doorbell").Scan(&note); err != nil {
		t.Fatalf("reading note: %v", err)
	}
	if note != "Limited — see docs" {
		t.Errorf("note = %q, want %q", note, "Limited — see docs")
	}
}

func TestListTTSPresets(t *testing.T) {
	d := newTestDB(t)
	presets := []TTSPreset{
		{Name: "beta", Endpoint: "http://beta:8080", Model: "kokoro", DefaultVoice: "af_sky", IsActive: false},
		{Name: "alpha", Endpoint: "http://alpha:8080", Model: "tts-1", DefaultVoice: "alloy", IsActive: false},
		{Name: "gamma", Endpoint: "http://gamma:8080", Model: "kokoro", DefaultVoice: "af_bella", IsActive: true},
	}
	for _, p := range presets {
		if err := SaveTTSPreset(d, p); err != nil {
			t.Fatalf("SaveTTSPreset %s: %v", p.Name, err)
		}
	}
	got, err := ListTTSPresets(d)
	if err != nil {
		t.Fatalf("ListTTSPresets: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(presets) = %d, want 3", len(got))
	}
	// Active preset should be first (ORDER BY is_active DESC, name)
	if got[0].Name != "gamma" {
		t.Errorf("first preset = %q, want %q (active first)", got[0].Name, "gamma")
	}
	if !got[0].IsActive {
		t.Errorf("first preset not active")
	}
	// Remaining sorted by name
	if got[1].Name != "alpha" {
		t.Errorf("second preset = %q, want %q", got[1].Name, "alpha")
	}
	if got[2].Name != "beta" {
		t.Errorf("third preset = %q, want %q", got[2].Name, "beta")
	}
	// Verify fields
	if got[0].Endpoint != "http://gamma:8080" {
		t.Errorf("gamma endpoint = %q, want %q", got[0].Endpoint, "http://gamma:8080")
	}
	if got[0].Model != "kokoro" {
		t.Errorf("gamma model = %q, want %q", got[0].Model, "kokoro")
	}
	if got[0].DefaultVoice != "af_bella" {
		t.Errorf("gamma voice = %q, want %q", got[0].DefaultVoice, "af_bella")
	}
}

func TestSaveTTSPresetActive(t *testing.T) {
	d := newTestDB(t)
	// Save an active preset
	p1 := TTSPreset{Name: "first", Endpoint: "http://first", Model: "kokoro", IsActive: true}
	if err := SaveTTSPreset(d, p1); err != nil {
		t.Fatalf("SaveTTSPreset first: %v", err)
	}
	// Save another active preset — should deactivate the first
	p2 := TTSPreset{Name: "second", Endpoint: "http://second", Model: "kokoro", IsActive: true}
	if err := SaveTTSPreset(d, p2); err != nil {
		t.Fatalf("SaveTTSPreset second: %v", err)
	}
	var activeCount int
	if err := d.QueryRow(`SELECT COUNT(*) FROM tts_presets WHERE is_active = 1`).Scan(&activeCount); err != nil {
		t.Fatalf("counting active: %v", err)
	}
	if activeCount != 1 {
		t.Errorf("active count = %d, want 1", activeCount)
	}
	var activeName string
	if err := d.QueryRow(`SELECT name FROM tts_presets WHERE is_active = 1`).Scan(&activeName); err != nil {
		t.Fatalf("reading active name: %v", err)
	}
	if activeName != "second" {
		t.Errorf("active preset = %q, want %q", activeName, "second")
	}
}

func TestSetActiveTTSPreset(t *testing.T) {
	d := newTestDB(t)
	p1 := TTSPreset{Name: "p1", Endpoint: "http://p1", Model: "kokoro", IsActive: true}
	p2 := TTSPreset{Name: "p2", Endpoint: "http://p2", Model: "kokoro", IsActive: false}
	if err := SaveTTSPreset(d, p1); err != nil {
		t.Fatalf("SaveTTSPreset p1: %v", err)
	}
	if err := SaveTTSPreset(d, p2); err != nil {
		t.Fatalf("SaveTTSPreset p2: %v", err)
	}
	// Activate p2 (the non-active one)
	if err := SetActiveTTSPreset(d, "p2"); err != nil {
		t.Fatalf("SetActiveTTSPreset: %v", err)
	}
	var activeName string
	if err := d.QueryRow(`SELECT name FROM tts_presets WHERE is_active = 1`).Scan(&activeName); err != nil {
		t.Fatalf("reading active: %v", err)
	}
	if activeName != "p2" {
		t.Errorf("active = %q, want %q", activeName, "p2")
	}
	var activeCount int
	if err := d.QueryRow(`SELECT COUNT(*) FROM tts_presets WHERE is_active = 1`).Scan(&activeCount); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if activeCount != 1 {
		t.Errorf("active count = %d, want 1", activeCount)
	}
}

func TestSetActiveTTSPresetNotFound(t *testing.T) {
	d := newTestDB(t)
	err := SetActiveTTSPreset(d, "nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent preset, got nil")
	}
}

func TestDeleteTTSPreset(t *testing.T) {
	d := newTestDB(t)
	// Save an active preset first
	active := TTSPreset{Name: "active", Endpoint: "http://active", Model: "kokoro", IsActive: true}
	if err := SaveTTSPreset(d, active); err != nil {
		t.Fatalf("SaveTTSPreset active: %v", err)
	}
	// Save a non-active preset
	other := TTSPreset{Name: "other", Endpoint: "http://other", Model: "kokoro", IsActive: false}
	if err := SaveTTSPreset(d, other); err != nil {
		t.Fatalf("SaveTTSPreset other: %v", err)
	}
	// Delete the non-active one
	if err := DeleteTTSPreset(d, "other"); err != nil {
		t.Fatalf("DeleteTTSPreset: %v", err)
	}
	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM tts_presets WHERE name = ?`, "other").Scan(&count); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if count != 0 {
		t.Errorf("row count = %d, want 0 (deleted)", count)
	}
}

func TestDeleteTTSPresetActive(t *testing.T) {
	d := newTestDB(t)
	active := TTSPreset{Name: "active", Endpoint: "http://active", Model: "kokoro", IsActive: true}
	if err := SaveTTSPreset(d, active); err != nil {
		t.Fatalf("SaveTTSPreset: %v", err)
	}
	err := DeleteTTSPreset(d, "active")
	if err == nil {
		t.Fatal("expected error deleting active preset, got nil")
	}
}

func TestSaveAndListVisionPrompts(t *testing.T) {
	d := newTestDB(t)
	prompts := []VisionPrompt{
		{Name: "zebra", Prompt: "Describe the zebra", Description: "zoo"},
		{Name: "apple", Prompt: "Describe the apple", Description: "fruit"},
		{Name: "mango", Prompt: "Describe the mango", Description: "tropical"},
	}
	for _, p := range prompts {
		if err := SaveVisionPrompt(d, p); err != nil {
			t.Fatalf("SaveVisionPrompt %s: %v", p.Name, err)
		}
	}
	got, err := ListVisionPrompts(d)
	if err != nil {
		t.Fatalf("ListVisionPrompts: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	// Ordered by name
	want := []string{"apple", "mango", "zebra"}
	for i, w := range want {
		if got[i].Name != w {
			t.Errorf("prompt[%d].Name = %q, want %q", i, got[i].Name, w)
		}
	}
	// Verify fields
	if got[0].Prompt != "Describe the apple" {
		t.Errorf("apple prompt = %q, want %q", got[0].Prompt, "Describe the apple")
	}
	if got[0].Description != "fruit" {
		t.Errorf("apple description = %q, want %q", got[0].Description, "fruit")
	}
}

func TestDeleteVisionPrompt(t *testing.T) {
	d := newTestDB(t)
	p := VisionPrompt{Name: "todelete", Prompt: "test", Description: ""}
	if err := SaveVisionPrompt(d, p); err != nil {
		t.Fatalf("SaveVisionPrompt: %v", err)
	}
	if err := DeleteVisionPrompt(d, "todelete"); err != nil {
		t.Fatalf("DeleteVisionPrompt: %v", err)
	}
	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM vision_prompts WHERE name = ?`, "todelete").Scan(&count); err != nil {
		t.Fatalf("counting: %v", err)
	}
	if count != 0 {
		t.Errorf("row count = %d, want 0 (deleted)", count)
	}
}

func TestSaveVisionPromptEmptyName(t *testing.T) {
	d := newTestDB(t)
	p := VisionPrompt{Name: "", Prompt: "test"}
	err := SaveVisionPrompt(d, p)
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestParseFilterJSON(t *testing.T) {
	t.Run("valid JSON", func(t *testing.T) {
		got := parseFilterJSON(`{"zone":"front","type":"person"}`)
		if got["zone"] != "front" {
			t.Errorf("zone = %q, want %q", got["zone"], "front")
		}
		if got["type"] != "person" {
			t.Errorf("type = %q, want %q", got["type"], "person")
		}
	})
	t.Run("empty string", func(t *testing.T) {
		got := parseFilterJSON("")
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})
	t.Run("empty object", func(t *testing.T) {
		got := parseFilterJSON("{}")
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})
	t.Run("invalid JSON", func(t *testing.T) {
		got := parseFilterJSON("{not valid json")
		if len(got) != 0 {
			t.Errorf("len = %d, want 0 (invalid JSON returns empty map)", len(got))
		}
	})
}

func TestParseCSV(t *testing.T) {
	t.Run("with values", func(t *testing.T) {
		got := parseCSV("front, back, side")
		if len(got) != 3 {
			t.Fatalf("len = %d, want 3", len(got))
		}
		if got[0] != "front" {
			t.Errorf("[0] = %q, want %q", got[0], "front")
		}
		if got[1] != "back" {
			t.Errorf("[1] = %q, want %q", got[1], "back")
		}
		if got[2] != "side" {
			t.Errorf("[2] = %q, want %q", got[2], "side")
		}
	})
	t.Run("empty string", func(t *testing.T) {
		got := parseCSV("")
		if got != nil {
			t.Errorf("got = %v, want nil", got)
		}
	})
	t.Run("whitespace trimmed", func(t *testing.T) {
		got := parseCSV("  a  ,  b  ")
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		if got[0] != "a" {
			t.Errorf("[0] = %q, want %q", got[0], "a")
		}
		if got[1] != "b" {
			t.Errorf("[1] = %q, want %q", got[1], "b")
		}
	})
}

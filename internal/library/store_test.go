package library

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// saveTestPreset creates a preset without ffmpeg by writing a dummy .raw file
// and inserting metadata directly into the database. This allows tests to run
// in CI environments where ffmpeg is not installed.
func saveTestPreset(t *testing.T, store *Store, category, name, text, voice string) *Preset {
	t.Helper()
	dir := filepath.Join(store.dir, category)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	rawPath := filepath.Join(dir, name+".raw")
	// Write 8000 bytes (1 second of G.711ulaw at 8kHz)
	dummyData := make([]byte, 8000)
	if err := os.WriteFile(rawPath, dummyData, 0o644); err != nil {
		t.Fatalf("write raw file failed: %v", err)
	}

	size := int64(len(dummyData))
	meta := Meta{
		Name:     name,
		Category: category,
		Text:     text,
		Voice:    voice,
		Duration: float64(size) / 8000,
		Size:     size,
		Created:  time.Now(),
	}

	_, err := store.db.Exec(
		`INSERT INTO presets (name, category, text, voice, duration, size, raw_path, created)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(category, name) DO UPDATE SET
		   text=excluded.text, voice=excluded.voice, duration=excluded.duration,
		   size=excluded.size, raw_path=excluded.raw_path, created=excluded.created`,
		meta.Name, meta.Category, meta.Text, meta.Voice,
		meta.Duration, meta.Size, rawPath, meta.Created,
	)
	if err != nil {
		t.Fatalf("insert preset metadata failed: %v", err)
	}

	return &Preset{Meta: meta, RawPath: rawPath}
}

func TestRenamePreset(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir, dir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	preset := saveTestPreset(t, store, "alerts", "test1", "hello", "voice1")
	oldPath := preset.RawPath

	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("old raw file missing: %v", err)
	}

	renamed, err := store.Rename("alerts", "test1", "alerts", "test2")
	if err != nil {
		t.Fatalf("Rename failed: %v", err)
	}
	if renamed.Name != "test2" {
		t.Errorf("renamed name = %q, want test2", renamed.Name)
	}
	if renamed.Category != "alerts" {
		t.Errorf("renamed category = %q, want alerts", renamed.Category)
	}

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("old raw file should not exist after rename")
	}

	newPath := filepath.Join(dir, "alerts", "test2.raw")
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("new raw file missing: %v", err)
	}

	_, err = store.Get("alerts", "test1")
	if err == nil {
		t.Error("old preset should not exist after rename")
	}

	p2, err := store.Get("alerts", "test2")
	if err != nil {
		t.Fatalf("Get renamed preset failed: %v", err)
	}
	if p2.Name != "test2" {
		t.Errorf("got name %q, want test2", p2.Name)
	}
}

func TestRenamePresetChangeCategory(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir, dir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	saveTestPreset(t, store, "alerts", "test1", "hello", "voice1")

	renamed, err := store.Rename("alerts", "test1", "warnings", "test1")
	if err != nil {
		t.Fatalf("Rename failed: %v", err)
	}
	if renamed.Category != "warnings" {
		t.Errorf("category = %q, want warnings", renamed.Category)
	}

	_, err = store.Get("warnings", "test1")
	if err != nil {
		t.Errorf("Get in new category failed: %v", err)
	}

	_, err = store.Get("alerts", "test1")
	if err == nil {
		t.Error("preset should not exist in old category")
	}
}

func TestRenamePresetConflict(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir, dir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	saveTestPreset(t, store, "alerts", "test1", "hello", "")
	saveTestPreset(t, store, "alerts", "test2", "world", "")

	_, err = store.Rename("alerts", "test1", "alerts", "test2")
	if err == nil {
		t.Error("Rename to existing name should fail")
	}

	_, err = store.Get("alerts", "test1")
	if err != nil {
		t.Errorf("original preset should still exist after failed rename: %v", err)
	}
}

func TestRenamePresetNotFound(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir, dir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	_, err = store.Rename("alerts", "nonexistent", "alerts", "newname")
	if err == nil {
		t.Error("Rename of nonexistent preset should fail")
	}
}

func TestRenamePresetNoOp(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir, dir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	saveTestPreset(t, store, "alerts", "test1", "hello", "")

	_, err = store.Rename("alerts", "test1", "alerts", "test1")
	if err != nil {
		t.Fatalf("no-op Rename failed: %v", err)
	}

	_, err = store.Get("alerts", "test1")
	if err != nil {
		t.Errorf("preset should still exist after no-op rename: %v", err)
	}
}

package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenamePreset(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir, dir)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// Create a preset
	wav := makeWAV(8000, 0.5) // 0.5s of silence
	preset, err := store.Save("alerts", "test1", "hello", "voice1", wav)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	oldPath := preset.RawPath

	// Verify file exists
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("old raw file missing: %v", err)
	}

	// Rename
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

	// Old file should be gone
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("old raw file should not exist after rename")
	}

	// New file should exist
	newPath := filepath.Join(dir, "alerts", "test2.raw")
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("new raw file missing: %v", err)
	}

	// Old name should not be findable
	_, err = store.Get("alerts", "test1")
	if err == nil {
		t.Error("old preset should not exist after rename")
	}

	// New name should be findable
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

	wav := makeWAV(8000, 0.3)
	_, err = store.Save("alerts", "test1", "hello", "voice1", wav)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Rename to different category
	renamed, err := store.Rename("alerts", "test1", "warnings", "test1")
	if err != nil {
		t.Fatalf("Rename failed: %v", err)
	}
	if renamed.Category != "warnings" {
		t.Errorf("category = %q, want warnings", renamed.Category)
	}

	// Should be findable in new category
	_, err = store.Get("warnings", "test1")
	if err != nil {
		t.Errorf("Get in new category failed: %v", err)
	}

	// Should not be in old category
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

	wav := makeWAV(8000, 0.3)
	_, err = store.Save("alerts", "test1", "hello", "", wav)
	if err != nil {
		t.Fatalf("Save test1 failed: %v", err)
	}
	_, err = store.Save("alerts", "test2", "world", "", wav)
	if err != nil {
		t.Fatalf("Save test2 failed: %v", err)
	}

	// Try to rename test1 → test2 (should fail, test2 exists)
	_, err = store.Rename("alerts", "test1", "alerts", "test2")
	if err == nil {
		t.Error("Rename to existing name should fail")
	}

	// Original should still exist
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

	wav := makeWAV(8000, 0.3)
	_, err = store.Save("alerts", "test1", "hello", "", wav)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Rename to same name/category (no-op)
	_, err = store.Rename("alerts", "test1", "alerts", "test1")
	if err != nil {
		t.Fatalf("no-op Rename failed: %v", err)
	}

	// Should still exist
	_, err = store.Get("alerts", "test1")
	if err != nil {
		t.Errorf("preset should still exist after no-op rename: %v", err)
	}
}

// makeWAV creates a minimal WAV file with the given sample rate and duration (seconds).
func makeWAV(sampleRate int, durationSec float64) []byte {
	numSamples := int(float64(sampleRate) * durationSec)
	dataSize := numSamples * 2 // 16-bit mono

	header := make([]byte, 44)
	copy(header[0:4], []byte("RIFF"))
	headerSize := 36 + dataSize
	header[4] = byte(headerSize)
	header[5] = byte(headerSize >> 8)
	header[6] = byte(headerSize >> 16)
	header[7] = byte(headerSize >> 24)
	copy(header[8:12], []byte("WAVE"))
	copy(header[12:16], []byte("fmt "))
	header[16] = 16 // fmt chunk size
	header[20] = 1  // PCM
	header[22] = 1  // mono
	header[24] = byte(sampleRate)
	header[25] = byte(sampleRate >> 8)
	header[26] = byte(sampleRate >> 16)
	header[27] = byte(sampleRate >> 24)
	byteRate := sampleRate * 2
	header[28] = byte(byteRate)
	header[29] = byte(byteRate >> 8)
	header[30] = byte(byteRate >> 16)
	header[31] = byte(byteRate >> 24)
	header[32] = 2  // block align
	header[34] = 16 // bits per sample
	copy(header[36:40], []byte("data"))
	header[40] = byte(dataSize)
	header[41] = byte(dataSize >> 8)
	header[42] = byte(dataSize >> 16)
	header[43] = byte(dataSize >> 24)

	return append(header, make([]byte, dataSize)...)
}

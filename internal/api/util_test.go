package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrependSilence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.raw")

	// Write a small "audio" file with known content.
	original := []byte{0x01, 0x02, 0x03, 0x04}
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}

	// Prepend 100ms of silence (800 bytes at 8000 bytes/sec).
	if err := prependSilence(path, 100); err != nil {
		t.Fatalf("prependSilence: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Should be 800 silence bytes + 4 original bytes.
	wantLen := 800 + len(original)
	if len(data) != wantLen {
		t.Fatalf("len = %d, want %d", len(data), wantLen)
	}

	// First 800 bytes should all be 0xFF (µ-law silence).
	for i := 0; i < 800; i++ {
		if data[i] != 0xFF {
			t.Fatalf("data[%d] = 0x%02X, want 0xFF (silence)", i, data[i])
		}
	}

	// Remaining bytes should match original.
	for i, b := range original {
		if data[800+i] != b {
			t.Fatalf("data[%d] = 0x%02X, want 0x%02X", 800+i, data[800+i], b)
		}
	}
}

func TestPrependSilenceToNewFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.raw")
	original := []byte{0xAA, 0xBB, 0xCC}
	if err := os.WriteFile(src, original, 0o644); err != nil {
		t.Fatal(err)
	}

	// With primeMs > 0, should create a new file.
	primed, err := prependSilenceToNewFile(src, dir, 50)
	if err != nil {
		t.Fatalf("prependSilenceToNewFile: %v", err)
	}
	if primed == src {
		t.Fatal("should have created a new file, got same path")
	}
	defer os.Remove(primed)

	data, err := os.ReadFile(primed)
	if err != nil {
		t.Fatal(err)
	}

	// 50ms = 400 bytes of silence + 3 original.
	wantLen := 400 + len(original)
	if len(data) != wantLen {
		t.Fatalf("len = %d, want %d", len(data), wantLen)
	}

	// Source file should be unchanged.
	srcData, _ := os.ReadFile(src)
	if len(srcData) != len(original) {
		t.Errorf("source file modified: len = %d, want %d", len(srcData), len(original))
	}

	// Clean up.
	t.Cleanup(func() { os.Remove(primed) })
}

func TestPrependSilenceToNewFileZeroMs(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.raw")
	if err := os.WriteFile(src, []byte{0x01}, 0o644); err != nil {
		t.Fatal(err)
	}

	// With primeMs = 0, should return src unchanged.
	result, err := prependSilenceToNewFile(src, dir, 0)
	if err != nil {
		t.Fatalf("prependSilenceToNewFile: %v", err)
	}
	if result != src {
		t.Errorf("with primeMs=0, should return src unchanged, got %q", result)
	}
}

package library

import (
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestIdentifierValidationAtStoreBoundary(t *testing.T) {
	store, err := NewStore(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	saveTestPreset(t, store, "alerts", "safe", "", "")
	for _, invalid := range []string{".", "..", "../outside", "a/b", `a\b`, "bad\x00name", "\n", " "} {
		t.Run(invalid, func(t *testing.T) {
			if _, err := store.Rename("alerts", "safe", invalid, "target"); err == nil {
				t.Error("rename accepted invalid category")
			}
			if _, err := store.Save("alerts", invalid, "", "", nil); err == nil {
				t.Error("save accepted invalid name")
			}
			if _, err := store.SaveFileWithProgressContext(context.Background(), invalid, "target", "missing", nil); err == nil {
				t.Error("upload accepted invalid category")
			}
			if _, err := store.SaveStream("alerts", invalid, "https://example.com"); err == nil {
				t.Error("stream accepted invalid name")
			}
			if err := store.Delete(invalid, "target"); err == nil {
				t.Error("delete accepted invalid category")
			}
		})
	}
	if _, err := store.Get("alerts", "safe"); err != nil {
		t.Fatalf("source changed after rejected operations: %v", err)
	}
}

func TestStoreRejectsSymlinkPaths(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	store, err := NewStore(dir, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	saveTestPreset(t, store, "alerts", "safe", "", "")
	if err := os.Symlink(outside, filepath.Join(dir, "linked")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rename("alerts", "safe", "linked", "escaped"); err == nil {
		t.Fatal("rename followed category symlink")
	}
	if _, err := store.SaveStream("linked", "escaped", "https://example.com"); err == nil {
		t.Fatal("stream accepted category symlink")
	}
	if err := os.Symlink(filepath.Join(outside, "victim.raw"), filepath.Join(dir, "alerts", "linked.raw")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rename("alerts", "safe", "alerts", "linked"); err == nil {
		t.Fatal("rename replaced destination symlink")
	}
}

func TestRenameDoesNotOverwriteOrphanFile(t *testing.T) {
	store, err := NewStore(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	saveTestPreset(t, store, "alerts", "safe", "", "")
	target := store.rawPath("alerts", "orphan")
	if err := os.WriteFile(target, []byte("keep me"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Rename("alerts", "safe", "alerts", "orphan"); err == nil {
		t.Fatal("rename overwrote orphan file")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "keep me" {
		t.Fatalf("orphan changed: %q, %v", data, err)
	}
}

func TestFileMutationsRollbackOnDatabaseError(t *testing.T) {
	for _, operation := range []string{"rename", "save", "delete", "stream"} {
		t.Run(operation, func(t *testing.T) {
			store, err := NewStore(t.TempDir(), t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			original := saveTestPreset(t, store, "alerts", "safe", "original", "")
			for _, event := range []string{"INSERT", "UPDATE", "DELETE"} {
				if _, err := store.db.Exec("CREATE TRIGGER fail_" + event + " BEFORE " + event + " ON presets BEGIN SELECT RAISE(FAIL, 'fixture'); END"); err != nil {
					t.Fatal(err)
				}
			}
			switch operation {
			case "rename":
				_, err = store.Rename("alerts", "safe", "alerts", "target")
			case "save":
				input := filepath.Join(t.TempDir(), "converted.raw")
				if err := os.WriteFile(input, []byte("replacement"), 0o644); err != nil {
					t.Fatal(err)
				}
				_, err = store.publishRaw("alerts", "safe", "replacement", "", input)
			case "delete":
				err = store.Delete("alerts", "safe")
			case "stream":
				_, err = store.SaveStream("alerts", "safe", "https://example.com")
			}
			if err == nil {
				t.Fatal("expected database rejection")
			}
			preset, err := store.Get("alerts", "safe")
			if err != nil || preset.Text != "original" {
				t.Fatalf("metadata changed: %+v, %v", preset, err)
			}
			data, err := os.ReadFile(original.RawPath)
			if err != nil || len(data) != 8000 || data[0] != 0 {
				t.Fatalf("original audio not restored: size=%d, err=%v", len(data), err)
			}
		})
	}
}

func TestGetRejectsStoredPathOutsideLibrary(t *testing.T) {
	store, err := NewStore(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	saveTestPreset(t, store, "alerts", "safe", "", "")
	if _, err := store.db.Exec(`UPDATE presets SET raw_path = ?`, filepath.Join(t.TempDir(), "outside.raw")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get("alerts", "safe"); err == nil {
		t.Fatal("read accepted external stored path")
	}
	if err := store.Delete("alerts", "safe"); err == nil {
		t.Fatal("delete accepted external stored path")
	}
}

func TestGetByNameRequiresCategoryWhenAmbiguous(t *testing.T) {
	store, err := NewStore(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	saveTestPreset(t, store, "alerts", "hello", "", "")
	saveTestPreset(t, store, "greetings", "hello", "", "")
	if _, err := store.GetByName("hello"); err == nil ||
		!strings.Contains(err.Error(), "specify a category") {
		t.Fatalf("ambiguous lookup must fail clearly: %v", err)
	}
	if _, err := store.Get("greetings", "hello"); err != nil {
		t.Fatal(err)
	}
}

func TestSaveFileTranscodesAndPreservesAudioOnFailure(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is unavailable")
	}
	store, err := NewStore(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	// A one-second mono PCM WAV provides deterministic input without another encoder.
	wav := make([]byte, 44+16000)
	copy(wav, "RIFF")
	binary.LittleEndian.PutUint32(wav[4:], uint32(len(wav)-8))
	copy(wav[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:], 16)
	binary.LittleEndian.PutUint16(wav[20:], 1)
	binary.LittleEndian.PutUint16(wav[22:], 1)
	binary.LittleEndian.PutUint32(wav[24:], 8000)
	binary.LittleEndian.PutUint32(wav[28:], 16000)
	binary.LittleEndian.PutUint16(wav[32:], 2)
	binary.LittleEndian.PutUint16(wav[34:], 16)
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[40:], 16000)
	src := filepath.Join(t.TempDir(), "input.wav")
	if err := os.WriteFile(src, wav, 0o644); err != nil {
		t.Fatal(err)
	}
	preset, err := store.SaveFile("uploads", "audio", src)
	if err != nil {
		t.Fatal(err)
	}
	if preset.Size != 8000 || preset.Duration != 1 {
		t.Fatalf("unexpected converted audio: %+v", preset.Meta)
	}
	if err := os.WriteFile(src, []byte("invalid audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveFile("uploads", "audio", src); err == nil {
		t.Fatal("expected invalid audio conversion to fail")
	}
	data, err := os.ReadFile(preset.RawPath)
	if err != nil || len(data) != 8000 || data[0] != 0xff {
		t.Fatalf("existing converted audio changed: size=%d, err=%v", len(data), err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.SaveFileWithProgressContext(ctx, "uploads", "audio", src, nil); err == nil {
		t.Fatal("canceled transcode succeeded")
	}
}

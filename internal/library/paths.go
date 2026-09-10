package library

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// ValidateIdentifier requires a category and name that each identify one path component.
func ValidateIdentifier(category, name string) error {
	for _, part := range []string{category, name} {
		if strings.TrimSpace(part) == "" || part == "." || part == ".." ||
			strings.ContainsAny(part, `/\`) || strings.IndexFunc(part, unicode.IsControl) >= 0 {
			return fmt.Errorf(
				"category and name must be nonempty path components without separators or control characters",
			)
		}
	}
	return nil
}

// checkPath rejects aliases as well as traversal; rooted operations also prevent symlink races escaping the library.
func (s *Store) checkPath(category, name string) error {
	if err := ValidateIdentifier(category, name); err != nil {
		return err
	}
	for _, path := range []string{category, filepath.Join(category, name+".raw")} {
		info, err := s.root.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("checking preset path: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("preset paths must not contain symbolic links")
		}
	}
	return nil
}

// validatePresetPath prevents stale or manipulated database paths escaping the library.
func (s *Store) validatePresetPath(p *Preset) error {
	if err := s.checkPath(p.Category, p.Name); err != nil {
		return err
	}
	if p.RawPath == "" {
		return nil
	}
	expected, err := filepath.Abs(s.rawPath(p.Category, p.Name))
	if err != nil {
		return fmt.Errorf("resolving preset path: %w", err)
	}
	actual, err := filepath.Abs(p.RawPath)
	if err != nil || actual != expected {
		return fmt.Errorf("stored audio path does not match preset identifier")
	}
	return nil
}

// saveTranscoded keeps ffmpeg output outside the library until conversion succeeds.
func (s *Store) saveTranscoded(ctx context.Context, category, name, text, voice, src string,
	normalize bool, progress func(float64),
) (*Preset, error) {
	if err := s.checkPath(category, name); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(s.tmpDir, "camspeak_transcode_*.raw")
	if err != nil {
		return nil, fmt.Errorf("creating transcode output: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("closing transcode output: %w", err)
	}
	if err := transcodeToRawWithProgress(ctx, src, tmp.Name(), normalize, progress); err != nil {
		return nil, fmt.Errorf("transcoding: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("saving canceled: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.publishRaw(category, name, text, voice, tmp.Name())
}

// publishRaw installs a complete file and restores its predecessor if metadata cannot be saved.
func (s *Store) publishRaw(category, name, text, voice, src string) (*Preset, error) {
	if err := s.checkPath(category, name); err != nil {
		return nil, err
	}
	if _, err := s.root.Lstat(filepath.Join(category, name+".raw")); err == nil {
		previous, err := s.Get(category, name)
		if err != nil || previous.RawPath == "" {
			return nil, fmt.Errorf(
				"audio file for %s/%s already exists without matching metadata",
				category,
				name,
			)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("checking existing audio: %w", err)
	}
	if err := s.root.MkdirAll(category, 0o755); err != nil {
		return nil, fmt.Errorf("creating category: %w", err)
	}
	in, err := os.Open(src)
	if err != nil {
		return nil, fmt.Errorf("opening converted audio: %w", err)
	}
	defer in.Close()
	stage := ".save-" + rand.Text()
	out, err := s.root.OpenFile(stage, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return nil, fmt.Errorf("staging converted audio: %w", err)
	}
	defer func() { _ = s.root.Remove(stage) }()
	_, copyErr := io.Copy(out, in)
	if err := errors.Join(copyErr, out.Close()); err != nil {
		return nil, fmt.Errorf("writing converted audio: %w", err)
	}
	raw := filepath.Join(category, name+".raw")
	backup := ".backup-" + rand.Text()
	hadPrevious := false
	if err := s.root.Link(raw, backup); err == nil {
		hadPrevious = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("preserving previous audio: %w", err)
	}
	restore := func() error {
		if hadPrevious {
			return s.root.Rename(backup, raw)
		}
		return nil
	}
	if err := s.root.Rename(stage, raw); err != nil {
		return nil, errors.Join(fmt.Errorf("installing converted audio: %w", err), restore())
	}
	preset, err := s.saveMeta(category, name, text, voice, s.rawPath(category, name))
	if err != nil {
		removeErr := s.root.Remove(raw)
		return nil, errors.Join(err, removeErr, restore())
	}
	if hadPrevious {
		if err := s.root.Remove(backup); err != nil {
			libraryLog.Warn("failed to remove replaced audio backup", "path", backup, "err", err)
		}
	}
	return preset, nil
}

// Package library manages pre-generated audio presets stored as G.711ulaw 8kHz raw files.
// Preset metadata is stored in SQLite; raw audio files remain on disk.
package library

import (
	"bufio"
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	clog "github.com/charmbracelet/log"

	"github.com/jeeftor/camspeak/internal/db"
	"github.com/jeeftor/camspeak/internal/logging"
	"github.com/jeeftor/camspeak/internal/util"
)

// Meta holds metadata for a preset alongside its .raw file.
type Meta struct {
	Name     string    `json:"name"`
	Category string    `json:"category"`
	Text     string    `json:"text,omitempty"`
	Voice    string    `json:"voice,omitempty"`
	URL      string    `json:"url,omitempty"` // live stream URL (stream presets only)
	Duration float64   `json:"duration"`      // seconds
	Size     int64     `json:"size"`          // bytes
	Gain     float64   `json:"gain"`          // per-preset gain multiplier (1.0 = no change)
	Created  time.Time `json:"created"`
}

// Preset is a ready-to-play audio clip.
type Preset struct {
	Meta

	RawPath string `json:"-"`
}

var libraryLog = logging.New("library", clog.InfoLevel)

// Store manages the preset library on disk + SQLite metadata.
type Store struct {
	dir    string
	tmpDir string
	db     *sql.DB
	root   *os.Root
	mu     sync.Mutex // serialize file/metadata mutations
}

// NewStore creates a Store rooted at dir (created if missing).
// A SQLite database is opened at dir/camspeak.db.
// Temp files are written to tmpDir (created if missing).
func NewStore(dir, tmpDir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating library dir: %w", err)
	}

	if tmpDir != "" {
		if err := os.MkdirAll(tmpDir, 0o755); err != nil {
			tmpDir = "" // fall back to os temp
		}
	}

	dbPath := filepath.Join(dir, "camspeak.db")

	database, err := db.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening preset database: %w", err)
	}

	root, err := os.OpenRoot(dir)
	if err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("opening library root: %w", err)
	}
	return &Store{dir: dir, tmpDir: tmpDir, db: database, root: root}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return errors.Join(s.db.Close(), s.root.Close())
}

// DB returns the underlying database connection (shared with event log).
func (s *Store) DB() *sql.DB {
	return s.db
}

// rawPath returns the .raw file path for a preset.
func (s *Store) rawPath(category, name string) string {
	return filepath.Join(s.dir, category, name+".raw")
}

// Save writes WAV bytes → G.711ulaw 8kHz raw via ffmpeg, plus metadata in SQLite.
func (s *Store) Save(category, name, text, voice string, wavData []byte) (*Preset, error) {
	if err := ValidateIdentifier(category, name); err != nil {
		return nil, err
	}

	// Write WAV to temp file
	tmp, err := os.CreateTemp(s.tmpDir, "camspeak_*.wav")
	if err != nil {
		return nil, fmt.Errorf("creating temp file: %w", err)
	}

	defer os.Remove(tmp.Name())
	defer tmp.Close()

	if _, err := tmp.Write(wavData); err != nil {
		return nil, fmt.Errorf("writing temp WAV: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("closing temp WAV: %w", err)
	}

	return s.saveTranscoded(
		context.Background(),
		category,
		name,
		text,
		voice,
		tmp.Name(),
		false,
		nil,
	)
}

// SaveFile transcodes any audio file (WAV/MP3/etc) to a preset.
func (s *Store) SaveFile(category, name string, srcFile string) (*Preset, error) {
	return s.SaveFileWithProgress(category, name, srcFile, nil)
}

// SaveStream creates a stream preset — a named live stream/playlist URL with
// no raw audio file on disk. When played, the URL is resolved (if a playlist)
// and streamed to the camera via ffmpeg in real time.
func (s *Store) SaveStream(category, name, url string) (*Preset, error) {
	if category == "" {
		category = "streams"
	}
	if err := ValidateIdentifier(category, name); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveMetaStream(category, name, url)
}

// saveMetaStream writes a stream preset's metadata to SQLite (no raw file).
// If overwriting an existing audio preset, the old .raw file is removed.
func (s *Store) saveMetaStream(category, name, url string) (*Preset, error) {
	if err := s.checkPath(category, name); err != nil {
		return nil, err
	}
	removeAudio := false
	if existing, err := s.Get(category, name); err == nil {
		removeAudio = existing.RawPath != ""
	}

	meta := Meta{
		Name:     name,
		Category: category,
		URL:      url,
		Created:  time.Now(),
	}

	_, err := s.db.Exec(
		`INSERT INTO presets (name, category, text, voice, url, duration, size, raw_path, created)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(category, name) DO UPDATE SET
		   text=excluded.text, voice=excluded.voice, url=excluded.url,
		   duration=excluded.duration, size=excluded.size,
		   raw_path=excluded.raw_path, created=excluded.created`,
		meta.Name, meta.Category, meta.Text, meta.Voice, meta.URL,
		meta.Duration, meta.Size, "", meta.Created,
	)
	if err != nil {
		return nil, fmt.Errorf("saving stream metadata: %w", err)
	}
	if removeAudio {
		if err := s.root.Remove(filepath.Join(category, name+".raw")); err != nil &&
			!errors.Is(err, os.ErrNotExist) {
			libraryLog.Warn(
				"failed to remove obsolete audio",
				"category",
				category,
				"name",
				name,
				"err",
				err,
			)
		}
	}

	return &Preset{Meta: meta}, nil
}

// SaveFileWithProgress is like SaveFile but calls progress with the
// transcoding percentage (0–100) as ffmpeg works. A nil progress callback
// is safe.
func (s *Store) SaveFileWithProgress(
	category, name, srcFile string,
	progress func(percent float64),
) (*Preset, error) {
	return s.SaveFileWithProgressContext(context.Background(), category, name, srcFile, progress)
}

// SaveFileWithProgressContext transcodes an upload with cancellation support.
func (s *Store) SaveFileWithProgressContext(ctx context.Context, category, name, srcFile string,
	progress func(float64),
) (*Preset, error) {
	return s.saveTranscoded(ctx, category, name, "", "", srcFile, true, progress)
}

// saveMeta writes preset metadata to SQLite and returns the Preset.
func (s *Store) saveMeta(category, name, text, voice, rawFile string) (*Preset, error) {
	info, err := s.root.Stat(filepath.Join(category, name+".raw"))
	if err != nil {
		return nil, fmt.Errorf("stat raw file: %w", err)
	}

	size := info.Size()
	duration := float64(size) / 8000

	meta := Meta{
		Name:     name,
		Category: category,
		Text:     text,
		Voice:    voice,
		Duration: duration,
		Size:     size,
		Gain:     1.0,
		Created:  time.Now(),
	}

	_, err = s.db.Exec(
		`INSERT INTO presets (name, category, text, voice, url, duration, size, raw_path, gain, created)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(category, name) DO UPDATE SET
		   text=excluded.text, voice=excluded.voice, url=excluded.url,
		   duration=excluded.duration, size=excluded.size,
		   raw_path=excluded.raw_path, gain=excluded.gain, created=excluded.created`,
		meta.Name,
		meta.Category,
		meta.Text,
		meta.Voice,
		"",
		meta.Duration,
		meta.Size,
		rawFile,
		meta.Gain,
		meta.Created,
	)
	if err != nil {
		return nil, fmt.Errorf("saving metadata: %w", err)
	}

	return &Preset{Meta: meta, RawPath: rawFile}, nil
}

// Get returns a preset by category/name.
func (s *Store) Get(category, name string) (*Preset, error) {
	if err := ValidateIdentifier(category, name); err != nil {
		return nil, err
	}
	var p Preset

	err := s.db.QueryRow(
		`SELECT name, category, text, voice, url, duration, size, raw_path, gain, created
		 FROM presets WHERE category = ? AND name = ?`,
		category, name,
	).Scan(&p.Name, &p.Category, &p.Text, &p.Voice, &p.URL,
		&p.Duration, &p.Size, &p.RawPath, &p.Gain, &p.Created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("preset %s/%s not found", category, name)
	}

	if err != nil {
		return nil, fmt.Errorf("querying preset: %w", err)
	}

	if err := s.validatePresetPath(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

// GetByName finds a preset by name alone (searches all categories).
func (s *Store) GetByName(name string) (*Preset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ValidateIdentifier("default", name); err != nil {
		return nil, err
	}
	var count int
	if err := s.db.QueryRow(`SELECT count(*) FROM presets WHERE name = ?`, name).Scan(&count); err != nil {
		return nil, fmt.Errorf("counting matching presets: %w", err)
	}
	if count > 1 {
		return nil, fmt.Errorf("preset %q exists in multiple categories; specify a category", name)
	}
	var p Preset

	err := s.db.QueryRow(
		`SELECT name, category, text, voice, url, duration, size, raw_path, gain, created
		 FROM presets WHERE name = ? LIMIT 1`,
		name,
	).Scan(&p.Name, &p.Category, &p.Text, &p.Voice, &p.URL,
		&p.Duration, &p.Size, &p.RawPath, &p.Gain, &p.Created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("preset %q not found", name)
	}

	if err != nil {
		return nil, fmt.Errorf("querying preset: %w", err)
	}

	if err := s.validatePresetPath(&p); err != nil {
		return nil, err
	}
	return &p, nil
}

// List returns all presets in the library.
func (s *Store) List() ([]Preset, error) {
	rows, err := s.db.Query(
		`SELECT name, category, text, voice, url, duration, size, raw_path, gain, created
		 FROM presets ORDER BY category, name`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing presets: %w", err)
	}
	defer rows.Close()

	presets := make([]Preset, 0)

	for rows.Next() {
		var p Preset
		err := rows.Scan(&p.Name, &p.Category, &p.Text, &p.Voice, &p.URL,
			&p.Duration, &p.Size, &p.RawPath, &p.Gain, &p.Created)
		if err != nil {
			return nil, fmt.Errorf("scanning preset row: %w", err)
		}

		presets = append(presets, p)
	}

	return presets, rows.Err()
}

// Delete removes a preset and its raw audio file (if it has one).
func (s *Store) Delete(category, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	preset, err := s.Get(category, name)
	if err != nil {
		return err
	}

	backup := ".delete-" + rand.Text()
	raw := filepath.Join(category, name+".raw")
	if preset.RawPath != "" {
		if err := s.root.Rename(raw, backup); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("staging preset deletion: %w", err)
		}
	}
	_, err = s.db.Exec(
		`DELETE FROM presets WHERE category = ? AND name = ?`,
		category, name,
	)
	if err != nil {
		var restoreErr error
		if preset.RawPath != "" {
			restoreErr = s.root.Rename(backup, raw)
		}
		return errors.Join(fmt.Errorf("deleting preset metadata: %w", err), restoreErr)
	}

	// Stream presets have no raw file on disk.
	if preset.RawPath != "" {
		if err := s.root.Remove(backup); err != nil && !errors.Is(err, os.ErrNotExist) {
			libraryLog.Warn("failed to remove raw file", "path", preset.RawPath, "err", err)
		}
	}

	return nil
}

// Rename moves a preset to a new name and/or category.
// The raw audio file is moved on disk (if it has one) and the SQLite row is
// updated. Returns an error if the source doesn't exist or the target already
// exists.
func (s *Store) Rename(oldCategory, oldName, newCategory, newName string) (*Preset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	preset, err := s.Get(oldCategory, oldName)
	if err != nil {
		return nil, err
	}

	// Normalize: if newCategory is empty, keep old; if newName is empty, keep old
	if newCategory == "" {
		newCategory = oldCategory
	}
	if newName == "" {
		newName = oldName
	}
	if err := ValidateIdentifier(newCategory, newName); err != nil {
		return nil, err
	}
	if err := s.checkPath(newCategory, newName); err != nil {
		return nil, err
	}

	// No-op if nothing changed
	if newCategory == oldCategory && newName == oldName {
		return preset, nil
	}
	if _, err := s.root.Lstat(filepath.Join(newCategory, newName+".raw")); err == nil {
		return nil, fmt.Errorf("audio file for %s/%s already exists", newCategory, newName)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("checking target audio: %w", err)
	}

	// Check that target doesn't already exist
	if _, err := s.Get(newCategory, newName); err == nil {
		return nil, fmt.Errorf("preset %s/%s already exists", newCategory, newName)
	}

	// Move the raw file (stream presets have no raw file)
	newRawPath := preset.RawPath
	if preset.RawPath != "" {
		if err := s.root.MkdirAll(newCategory, 0o755); err != nil {
			return nil, fmt.Errorf("creating target category dir: %w", err)
		}

		newRawPath = s.rawPath(newCategory, newName)
		// Link fails if the destination already exists, including orphan files.
		if err := s.root.Link(filepath.Join(oldCategory, oldName+".raw"), filepath.Join(newCategory, newName+".raw")); err != nil {
			return nil, fmt.Errorf("moving raw file: %w", err)
		}
	}

	// Update the database row
	_, err = s.db.Exec(
		`UPDATE presets SET name = ?, category = ?, raw_path = ? WHERE category = ? AND name = ?`,
		newName, newCategory, newRawPath, oldCategory, oldName,
	)
	if err != nil {
		// Try to move the file back on DB failure
		if preset.RawPath != "" && newRawPath != preset.RawPath {
			if removeErr := s.root.Remove(filepath.Join(newCategory, newName+".raw")); removeErr != nil {
				return nil, errors.Join(fmt.Errorf("updating preset metadata: %w", err), removeErr)
			}
		}
		return nil, fmt.Errorf("updating preset metadata: %w", err)
	}
	if preset.RawPath != "" {
		if err := s.root.Remove(filepath.Join(oldCategory, oldName+".raw")); err != nil {
			libraryLog.Warn(
				"failed to remove old preset link",
				"category",
				oldCategory,
				"name",
				oldName,
				"err",
				err,
			)
		}
	}

	preset.Name = newName
	preset.Category = newCategory
	preset.RawPath = newRawPath
	return preset, nil
}

// GetRawPath returns the path to the raw file for streaming.
func (p *Preset) GetRawPath() string {
	return p.RawPath
}

// SetGain updates the per-preset gain multiplier in the database.
func (s *Store) SetGain(category, name string, gain float64) error {
	if err := ValidateIdentifier(category, name); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`UPDATE presets SET gain = ? WHERE category = ? AND name = ?`,
		gain, category, name,
	)
	if err != nil {
		return fmt.Errorf("updating preset gain: %w", err)
	}
	return nil
}

// ComputeRMS reads the raw G.711 µ-law file and returns the true RMS of
// all decoded samples, normalized to 0.0-1.0 (relative to max µ-law value).
func ComputeRMS(rawPath string) (float64, error) {
	rawBytes, err := os.ReadFile(rawPath)
	if err != nil {
		return 0, fmt.Errorf("reading raw file: %w", err)
	}
	if len(rawBytes) == 0 {
		return 0, nil
	}

	var sumSq float64
	for _, b := range rawBytes {
		pcm := float64(util.MulawDecode(b))
		sumSq += pcm * pcm
	}

	rms := math.Sqrt(sumSq / float64(len(rawBytes)))
	// Normalize to 0.0-1.0 (max µ-law value is 32124)
	normalized := rms / 32124.0
	if normalized > 1.0 {
		normalized = 1.0
	}
	return normalized, nil
}

// IsStream returns true if this preset is a live stream URL rather than a
// raw audio file on disk.
func (p *Preset) IsStream() bool {
	return p.URL != ""
}

// probeDuration returns the duration of an audio file in seconds via ffprobe.
// Returns 0 if ffprobe is unavailable or the duration can't be determined
// (in which case progress reporting falls back to indeterminate).
func probeDuration(ctx context.Context, src string) float64 {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "csv=p=0",
		src,
	)
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	d, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil || d <= 0 {
		return 0
	}
	return d
}

// transcodeToRawWithProgress converts audio to G.711ulaw and reports transcoding
// progress via the callback (percent 0–100). If the input duration can't be
// probed, the callback is called with -1 (indeterminate) once at the start.
// A nil callback is safe.
func transcodeToRawWithProgress(
	ctx context.Context, src, dst string, normalize bool,
	progress func(percent float64),
) error {
	af := "volume=3.0"
	if normalize {
		af = "loudnorm=I=-16:TP=-1.5:LRA=11"
	}

	totalDuration := probeDuration(ctx, src)
	if progress != nil {
		if totalDuration > 0 {
			progress(0)
		} else {
			progress(-1) // indeterminate
		}
	}

	cmd := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-y",
		"-nostdin",
		"-hide_banner",
		"-loglevel",
		"error",
		"-i",
		src,
		"-af",
		af,
		"-ar",
		"8000",
		"-ac",
		"1",
		"-c:a",
		"pcm_mulaw",
		"-f",
		"mulaw",
		"-progress",
		"-", // progress key=value lines to stdout
		dst,
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg stdout pipe: %w", err)
	}
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start: %w", err)
	}

	if progress != nil && totalDuration > 0 {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			// Progress lines look like: out_time_us=16000000
			if strings.HasPrefix(line, "out_time_us=") {
				us, err := strconv.ParseInt(strings.TrimPrefix(line, "out_time_us="), 10, 64)
				if err != nil || us < 0 {
					continue
				}
				pct := float64(us) / 1_000_000 / totalDuration * 100
				if pct > 100 {
					pct = 100
				}
				progress(pct)
			} else if line == "progress=end" {
				progress(100)
			}
		}
	} else {
		// No progress callback or no duration — drain stdout.
		_, _ = io.Copy(io.Discard, stdout)
	}

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg: %w\n%s", err, stderrBuf.String())
	}

	return nil
}

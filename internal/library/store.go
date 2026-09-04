// Package library manages pre-generated audio presets stored as G.711ulaw 8kHz raw files.
// Preset metadata is stored in SQLite; raw audio files remain on disk.
package library

import (
	"bufio"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	clog "github.com/charmbracelet/log"

	"github.com/jeeftor/camspeak/internal/db"
	"github.com/jeeftor/camspeak/internal/logging"
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
	Created  time.Time `json:"created"`
}

// Preset is a ready-to-play audio clip.
type Preset struct {
	Meta

	RawPath string `json:"-"`
}

var log = logging.New("library", clog.InfoLevel)

// Store manages the preset library on disk + SQLite metadata.
type Store struct {
	dir    string
	tmpDir string
	db     *sql.DB
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

	return &Store{dir: dir, tmpDir: tmpDir, db: database}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
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
	dir := filepath.Join(s.dir, category)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("creating category dir: %w", err)
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

	rawFile := s.rawPath(category, name)
	if err := transcodeToRaw(tmp.Name(), rawFile, false); err != nil {
		return nil, fmt.Errorf("transcoding to G.711ulaw: %w", err)
	}

	return s.saveMeta(category, name, text, voice, rawFile)
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
	return s.saveMetaStream(category, name, url)
}

// saveMetaStream writes a stream preset's metadata to SQLite (no raw file).
// If overwriting an existing audio preset, the old .raw file is removed.
func (s *Store) saveMetaStream(category, name, url string) (*Preset, error) {
	// Check if we're overwriting an audio preset and clean up its raw file.
	if existing, err := s.Get(category, name); err == nil && existing.RawPath != "" {
		if err := os.Remove(existing.RawPath); err != nil {
			log.Warn("saveMetaStream: failed to remove orphaned raw file", "path", existing.RawPath, "err", err)
		}
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

	return &Preset{Meta: meta}, nil
}

// SaveFileWithProgress is like SaveFile but calls progress with the
// transcoding percentage (0–100) as ffmpeg works. A nil progress callback
// is safe.
func (s *Store) SaveFileWithProgress(
	category, name, srcFile string,
	progress func(percent float64),
) (*Preset, error) {
	dir := filepath.Join(s.dir, category)
	err := os.MkdirAll(dir, 0o755)
	if err != nil {
		return nil, fmt.Errorf("creating category dir: %w", err)
	}

	rawFile := s.rawPath(category, name)
	err = transcodeToRawWithProgress(srcFile, rawFile, true, progress)
	if err != nil {
		return nil, fmt.Errorf("transcoding: %w", err)
	}

	return s.saveMeta(category, name, "", "", rawFile)
}

// saveMeta writes preset metadata to SQLite and returns the Preset.
func (s *Store) saveMeta(category, name, text, voice, rawFile string) (*Preset, error) {
	info, err := os.Stat(rawFile)
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
		Created:  time.Now(),
	}

	_, err = s.db.Exec(
		`INSERT INTO presets (name, category, text, voice, url, duration, size, raw_path, created)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(category, name) DO UPDATE SET
		   text=excluded.text, voice=excluded.voice, url=excluded.url,
		   duration=excluded.duration, size=excluded.size,
		   raw_path=excluded.raw_path, created=excluded.created`,
		meta.Name, meta.Category, meta.Text, meta.Voice, "",
		meta.Duration, meta.Size, rawFile, meta.Created,
	)
	if err != nil {
		return nil, fmt.Errorf("saving metadata: %w", err)
	}

	return &Preset{Meta: meta, RawPath: rawFile}, nil
}

// Get returns a preset by category/name.
func (s *Store) Get(category, name string) (*Preset, error) {
	var p Preset

	err := s.db.QueryRow(
		`SELECT name, category, text, voice, url, duration, size, raw_path, created
		 FROM presets WHERE category = ? AND name = ?`,
		category, name,
	).Scan(&p.Name, &p.Category, &p.Text, &p.Voice, &p.URL,
		&p.Duration, &p.Size, &p.RawPath, &p.Created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("preset %s/%s not found", category, name)
	}

	if err != nil {
		return nil, fmt.Errorf("querying preset: %w", err)
	}

	return &p, nil
}

// GetByName finds a preset by name alone (searches all categories).
func (s *Store) GetByName(name string) (*Preset, error) {
	var p Preset

	err := s.db.QueryRow(
		`SELECT name, category, text, voice, url, duration, size, raw_path, created
		 FROM presets WHERE name = ? LIMIT 1`,
		name,
	).Scan(&p.Name, &p.Category, &p.Text, &p.Voice, &p.URL,
		&p.Duration, &p.Size, &p.RawPath, &p.Created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("preset %q not found", name)
	}

	if err != nil {
		return nil, fmt.Errorf("querying preset: %w", err)
	}

	return &p, nil
}

// List returns all presets in the library.
func (s *Store) List() ([]Preset, error) {
	rows, err := s.db.Query(
		`SELECT name, category, text, voice, url, duration, size, raw_path, created
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
			&p.Duration, &p.Size, &p.RawPath, &p.Created)
		if err != nil {
			return nil, fmt.Errorf("scanning preset row: %w", err)
		}

		presets = append(presets, p)
	}

	return presets, rows.Err()
}

// Delete removes a preset and its raw audio file (if it has one).
func (s *Store) Delete(category, name string) error {
	preset, err := s.Get(category, name)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(
		`DELETE FROM presets WHERE category = ? AND name = ?`,
		category, name,
	)
	if err != nil {
		return fmt.Errorf("deleting preset metadata: %w", err)
	}

	// Stream presets have no raw file on disk.
	if preset.RawPath != "" {
		if err := os.Remove(preset.RawPath); err != nil {
			log.Warn("failed to remove raw file", "path", preset.RawPath, "err", err)
		}
	}

	return nil
}

// Rename moves a preset to a new name and/or category.
// The raw audio file is moved on disk (if it has one) and the SQLite row is
// updated. Returns an error if the source doesn't exist or the target already
// exists.
func (s *Store) Rename(oldCategory, oldName, newCategory, newName string) (*Preset, error) {
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

	// No-op if nothing changed
	if newCategory == oldCategory && newName == oldName {
		return preset, nil
	}

	// Check that target doesn't already exist
	if _, err := s.Get(newCategory, newName); err == nil {
		return nil, fmt.Errorf("preset %s/%s already exists", newCategory, newName)
	}

	// Move the raw file (stream presets have no raw file)
	newRawPath := preset.RawPath
	if preset.RawPath != "" {
		newDir := filepath.Join(s.dir, newCategory)
		if err := os.MkdirAll(newDir, 0o755); err != nil {
			return nil, fmt.Errorf("creating target category dir: %w", err)
		}

		newRawPath = s.rawPath(newCategory, newName)
		if err := os.Rename(preset.RawPath, newRawPath); err != nil {
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
			_ = os.Rename(newRawPath, preset.RawPath)
		}
		return nil, fmt.Errorf("updating preset metadata: %w", err)
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

// IsStream returns true if this preset is a live stream URL rather than a
// raw audio file on disk.
func (p *Preset) IsStream() bool {
	return p.URL != ""
}

// transcodeToRaw converts any audio file to G.711ulaw 8kHz raw via ffmpeg.
// When normalize is true, the uploaded audio is loudness-normalized so it
// sits in the same volume range as TTS-generated clips. When false, a 3x
// volume boost is applied (used for TTS output that is already consistent).
func transcodeToRaw(src, dst string, normalize bool) error {
	af := "volume=3.0"
	if normalize {
		af = "loudnorm=I=-16:TP=-1.5:LRA=11"
	}
	cmd := exec.Command("ffmpeg", "-y",
		"-i", src,
		"-af", af,
		"-ar", "8000",
		"-ac", "1",
		"-c:a", "pcm_mulaw",
		"-f", "mulaw",
		dst,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg: %w\n%s", err, out)
	}

	return nil
}

// probeDuration returns the duration of an audio file in seconds via ffprobe.
// Returns 0 if ffprobe is unavailable or the duration can't be determined
// (in which case progress reporting falls back to indeterminate).
func probeDuration(src string) float64 {
	cmd := exec.Command("ffprobe",
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

// transcodeToRawWithProgress is like transcodeToRaw but reports transcoding
// progress via the callback (percent 0–100). If the input duration can't be
// probed, the callback is called with -1 (indeterminate) once at the start.
// A nil callback is safe.
func transcodeToRawWithProgress(
	src, dst string, normalize bool,
	progress func(percent float64),
) error {
	af := "volume=3.0"
	if normalize {
		af = "loudnorm=I=-16:TP=-1.5:LRA=11"
	}

	totalDuration := probeDuration(src)
	if progress != nil {
		if totalDuration > 0 {
			progress(0)
		} else {
			progress(-1) // indeterminate
		}
	}

	cmd := exec.Command("ffmpeg", "-y",
		"-i", src,
		"-af", af,
		"-ar", "8000",
		"-ac", "1",
		"-c:a", "pcm_mulaw",
		"-f", "mulaw",
		"-progress", "-", // progress key=value lines to stdout
		dst,
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start: %w", err)
	}

	// Read progress from stdout, collect stderr for error reporting.
	var stderrBuf strings.Builder
	go func() {
		_, _ = io.Copy(&stderrBuf, stderr)
	}()

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

// Package db provides SQLite database initialization and schema management.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Schema defines the database tables.
const schema = `
CREATE TABLE IF NOT EXISTS presets (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    name     TEXT    NOT NULL,
    category TEXT    NOT NULL DEFAULT 'default',
    text     TEXT    DEFAULT '',
    voice    TEXT    DEFAULT '',
    duration REAL    DEFAULT 0,
    size     INTEGER DEFAULT 0,
    raw_path TEXT    NOT NULL,
    created  DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(category, name)
);

CREATE TABLE IF NOT EXISTS events (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    camera  TEXT    NOT NULL,
    action  TEXT    NOT NULL,
    text    TEXT    DEFAULT '',
    created DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS preferences (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tts_presets (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT UNIQUE NOT NULL,
    endpoint      TEXT NOT NULL,
    model         TEXT NOT NULL DEFAULT 'kokoro',
    api_key       TEXT DEFAULT '',
    default_voice TEXT DEFAULT 'af_sky',
    description   TEXT DEFAULT '',
    is_active     INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS cameras (
    name          TEXT PRIMARY KEY,
    type          TEXT NOT NULL,
    ip            TEXT NOT NULL,
    user          TEXT DEFAULT '',
    pass          TEXT DEFAULT '',
    channel       INTEGER DEFAULT 1,
    stream        TEXT DEFAULT '',
    enabled       INTEGER DEFAULT 0,
    vision_prompt TEXT DEFAULT ''
);

CREATE TABLE IF NOT EXISTS rules (
    id      INTEGER PRIMARY KEY AUTOINCREMENT,
    topic   TEXT NOT NULL DEFAULT 'frigate/events',
    filter  TEXT DEFAULT '{}',
    cameras TEXT DEFAULT '',
    preset  TEXT DEFAULT '',
    text    TEXT DEFAULT '',
    voice   TEXT DEFAULT '',
    enabled INTEGER DEFAULT 1
);

CREATE TABLE IF NOT EXISTS vision_prompts (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT UNIQUE NOT NULL,
    prompt      TEXT NOT NULL DEFAULT '',
    description TEXT DEFAULT '',
    created     DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_presets_category ON presets(category);
CREATE INDEX IF NOT EXISTS idx_events_created ON events(created DESC);
CREATE INDEX IF NOT EXISTS idx_tts_presets_active ON tts_presets(is_active);
CREATE INDEX IF NOT EXISTS idx_rules_enabled ON rules(enabled);
`

// Open opens (or creates) the SQLite database at dbPath.
// The parent directory is created if it does not exist.
func Open(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		err := os.MkdirAll(dir, 0o755)
		if err != nil {
			return nil, fmt.Errorf("creating db directory: %w", err)
		}
	}

	dsn := "file:" + dbPath + "?_journal_mode=WAL&_busy_timeout=5000"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()

		return nil, fmt.Errorf("pinging database: %w", err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()

		return nil, fmt.Errorf("initializing schema: %w", err)
	}

	// Migrations: add columns that may not exist in older databases.
	migrate(db)

	return db, nil
}

// migrate applies incremental schema changes for existing databases.
func migrate(db *sql.DB) {
	// Add 'stream' column to cameras if missing (added in v1.4.0).
	var streamCol int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('cameras') WHERE name='stream'`).Scan(&streamCol); err == nil &&
		streamCol == 0 {
		_, _ = db.Exec(`ALTER TABLE cameras ADD COLUMN stream TEXT DEFAULT ''`)
	}
	// Add 'enabled' column to cameras if missing (added in v1.4.5).
	// Default 0 (disabled) — users must explicitly enable cameras.
	var enabledCol int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('cameras') WHERE name='enabled'`).Scan(&enabledCol); err == nil &&
		enabledCol == 0 {
		_, _ = db.Exec(`ALTER TABLE cameras ADD COLUMN enabled INTEGER DEFAULT 0`)
	}
	// Add 'vision_prompt' column to cameras if missing (added in v1.8.0).
	var vpCol int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('cameras') WHERE name='vision_prompt'`).Scan(&vpCol); err == nil &&
		vpCol == 0 {
		_, _ = db.Exec(`ALTER TABLE cameras ADD COLUMN vision_prompt TEXT DEFAULT ''`)
	}
	// Add 'airplay_enabled' column to cameras if missing (added in v1.9.0).
	// Default 1 (enabled) — existing cameras automatically get AirPlay.
	var apCol int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('cameras') WHERE name='airplay_enabled'`).Scan(&apCol); err == nil &&
		apCol == 0 {
		_, _ = db.Exec(`ALTER TABLE cameras ADD COLUMN airplay_enabled INTEGER DEFAULT 1`)
	}
}

// Package db provides SQLite database initialization and schema management.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	clog "github.com/charmbracelet/log"

	"github.com/jeeftor/camspeak/internal/logging"
	_ "modernc.org/sqlite"
)

var log = logging.New("db", clog.InfoLevel)

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
    vision_prompt TEXT DEFAULT '',
    airplay_enabled INTEGER DEFAULT 1,
    airplay_name  TEXT DEFAULT '',
    airplay_model TEXT DEFAULT '',
    gain          REAL DEFAULT 3.0,
    note          TEXT DEFAULT ''
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

	dsn := "file:" + dbPath + "?_journal_mode=WAL&_busy_timeout=15000&_txlock=immediate"

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// SQLite only supports a single writer. Limit the connection pool to 1
	// so all queries are serialized through one connection and we never hit
	// SQLITE_BUSY from concurrent writes on separate connections.
	db.SetMaxOpenConns(1)

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
	// addColumn adds a column to a table if it doesn't already exist.
	// Returns true if the column was added (migration ran).
	addColumn := func(table, column, def string) bool {
		var col int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name=?`,
			table, column,
		).Scan(&col); err != nil {
			log.Warn("migration: failed to check column", "table", table, "column", column, "err", err)
			return false
		}
		if col > 0 {
			return false
		}
		stmt := fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, def)
		if _, err := db.Exec(stmt); err != nil {
			log.Warn("migration: failed to add column", "table", table, "column", column, "err", err)
			return false
		}
		log.Info("migration: added column", "table", table, "column", column)
		return true
	}

	// Add 'stream' column to cameras if missing (added in v1.4.0).
	addColumn("cameras", "stream", "TEXT DEFAULT ''")
	// Add 'enabled' column to cameras if missing (added in v1.4.5).
	// Default 0 (disabled) — users must explicitly enable cameras.
	addColumn("cameras", "enabled", "INTEGER DEFAULT 0")
	// Add 'vision_prompt' column to cameras if missing (added in v1.8.0).
	addColumn("cameras", "vision_prompt", "TEXT DEFAULT ''")
	// Add 'airplay_enabled' column to cameras if missing (added in v1.9.0).
	// Default 1 (enabled) — existing cameras automatically get AirPlay.
	addColumn("cameras", "airplay_enabled", "INTEGER DEFAULT 1")
	// Add 'airplay_name' column to cameras if missing (added in v2.3.17).
	addColumn("cameras", "airplay_name", "TEXT DEFAULT ''")
	// Add 'airplay_model' column to cameras if missing (added in v2.3.34).
	addColumn("cameras", "airplay_model", "TEXT DEFAULT ''")
	// Add 'gain' column to cameras if missing (added in v2.3.40).
	addColumn("cameras", "gain", "REAL DEFAULT 3.0")
	// Add 'note' column to cameras if missing (added in v2.4.0).
	// Stores per-camera limitation warnings (e.g. Reolink "Limited" tag).
	addColumn("cameras", "note", "TEXT DEFAULT ''")
	// Add 'vision_stream' column to cameras if missing (added in v2.10.0).
	// go2rtc stream name for vision snapshots (e.g. "frontyard_sub"); empty = Frigate detect.
	addColumn("cameras", "vision_stream", "TEXT DEFAULT ''")
	// Add 'vision_width' column to cameras if missing (added in v2.10.0).
	// Max width in pixels for vision snapshots (0 = no resize).
	addColumn("cameras", "vision_width", "INTEGER DEFAULT 0")
	// Add 'voice' column to events if missing (added in v2.13.0).
	addColumn("events", "voice", "TEXT DEFAULT ''")
	// Add 'url' column to presets if missing (added in v2.14.0).
	// Stream presets store a live stream/playlist URL here; audio presets
	// leave it empty and use raw_path instead.
	addColumn("presets", "url", "TEXT DEFAULT ''")
	// Add 'gain' column to presets if missing (added in v2.19.0).
	// Per-preset gain multiplier (1.0 = no change). Applied at send time
	// in addition to the camera's gain. Auto-calculated from RMS or
	// manually adjusted from the library UI.
	addColumn("presets", "gain", "REAL DEFAULT 1.0")
	// Add 'snap_method' column to cameras if missing (added in v2.19.5).
	// Preferred snapshot method: "auto", "isapi", "go2rtc", "frigate".
	addColumn("cameras", "snap_method", "TEXT DEFAULT ''")
}

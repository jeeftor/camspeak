// Package cmd defines the cobra CLI for camspeak.
package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	clog "github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	"github.com/jeeftor/camspeak/internal/db"
)

var (
	dataDir string
	version = "dev"
	appLog  = clog.NewWithOptions(os.Stderr, clog.Options{
		ReportTimestamp: true,
		Level:           logLevel(),
	})
)

// logLevel returns the log level from CAMSPEAK_LOG_LEVEL env var,
// defaulting to Info. Valid values: debug, info, warn, error.
func logLevel() clog.Level {
	switch strings.ToLower(os.Getenv("CAMSPEAK_LOG_LEVEL")) {
	case "debug":
		return clog.DebugLevel
	case "warn":
		return clog.WarnLevel
	case "error":
		return clog.ErrorLevel
	default:
		return clog.InfoLevel
	}
}

// SetVersion sets the application version (called from main.go).
func SetVersion(v string) {
	version = v
}

var rootCmd = &cobra.Command{
	Use:   "camspeak",
	Short: "Camera audio router — stream TTS and audio to camera speakers",
	Long: `camspeak routes text-to-speech and audio files to Hikvision and Reolink
camera speakers via ISAPI two-way audio and Kokoro TTS.

Configuration is stored in SQLite at <data-dir>/camspeak.db.
Env vars (CAMSPEAK_*) override database preferences.`,
}

// Execute runs the root cobra command.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

// resolveDataDir returns the data directory from flag, env, or default.
func resolveDataDir() string {
	if dataDir != "" {
		return dataDir
	}
	if v := os.Getenv("CAMSPEAK_DATA_DIR"); v != "" {
		return v
	}
	return "./data"
}

// openDB opens the SQLite database at <data-dir>/camspeak.db.
func openDB() (*sql.DB, string, error) {
	dir := resolveDataDir()
	dbPath := filepath.Join(dir, "camspeak.db")
	database, err := db.Open(dbPath)
	if err != nil {
		return nil, dir, fmt.Errorf("opening database: %w", err)
	}
	return database, dir, nil
}

func init() {
	rootCmd.PersistentFlags().
		StringVar(&dataDir, "data-dir", "", "data directory (default: ./data or CAMSPEAK_DATA_DIR env)")
}

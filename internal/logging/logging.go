// Package logging provides a shared charmbracelet/log factory for camspeak.
// All subsystem loggers should be created via New rather than clog.NewWithOptions
// directly, to ensure consistent timestamp reporting and colored prefixes.
package logging

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	clog "github.com/charmbracelet/log"
)

// subsystemColors maps prefix stems (lowercase) to terminal colors.
// Stems are matched as a prefix of the logger name so that e.g. "airplay"
// matches both "airplay" and "airplay[backyard]".
var subsystemColors = []struct {
	stem  string
	color lipgloss.Color
}{
	{"api", lipgloss.Color("#38BDF8")},       // sky
	{"airplay", lipgloss.Color("#A78BFA")},   // violet
	{"shairport", lipgloss.Color("#C084FC")}, // purple
	{"mqtt", lipgloss.Color("#FB923C")},      // orange
	{"hikvision", lipgloss.Color("#4ADE80")}, // green
	{"go2rtc", lipgloss.Color("#4ADE80")},    // green
	{"reolink", lipgloss.Color("#4ADE80")},   // green
	{"onvif", lipgloss.Color("#4ADE80")},     // green
	{"cameras", lipgloss.Color("#4ADE80")},   // green
}

// New creates a charmbracelet/log Logger with timestamp reporting, a
// subsystem-colored bold prefix, and the given minimum log level.
func New(prefix string, level clog.Level) *clog.Logger {
	l := clog.NewWithOptions(os.Stderr, clog.Options{
		Prefix:          prefix,
		ReportTimestamp: true,
		Level:           level,
	})
	if prefix != "" {
		styles := clog.DefaultStyles()
		styles.Prefix = lipgloss.NewStyle().
			Foreground(colorForPrefix(prefix)).
			Bold(true)
		l.SetStyles(styles)
	}
	return l
}

// colorForPrefix returns the display color for a given logger prefix stem.
func colorForPrefix(prefix string) lipgloss.Color {
	lower := strings.ToLower(prefix)
	for _, s := range subsystemColors {
		if strings.HasPrefix(lower, s.stem) {
			return s.color
		}
	}
	return lipgloss.Color("#94A3B8") // slate default
}

package airplay

import clog "github.com/charmbracelet/log"

// Receiver is the common interface implemented by both the pure-Go Server
// and the shairport-sync-backed ShairportServer.
type Receiver interface {
	Start() error
	Stop()
	SetLogLevel(level clog.Level)
}

// Compile-time interface satisfaction checks.
var (
	_ Receiver = (*Server)(nil)
	_ Receiver = (*ShairportServer)(nil)
)

# camspeak — Claude Code Instructions

## Logging

**Use `github.com/charmbracelet/log` (aliased as `clog`), NOT stdlib `log/slog`.**

This overrides the global CLAUDE.md preference for `slog`. The project uses charmbracelet/log
throughout for colorized, structured terminal output.

Rules:
- Always import as `clog "github.com/charmbracelet/log"`
- Every `clog.NewWithOptions` call **must** include `ReportTimestamp: true` for consistent log format
- Use `clog.InfoLevel` (not `slog.LevelInfo`, etc.)
- Pass `Level: apiLogLevel` (or the appropriate package-level variable) so log level is configurable

Example:
```go
log: clog.NewWithOptions(os.Stderr, clog.Options{
    Prefix:          "mypackage",
    ReportTimestamp: true,
    Level:           clog.InfoLevel,
}),
```

## Stack

- **Backend**: Go, Echo v4, SQLite (via database/sql)
- **Frontend**: Svelte 5 + Vite + Tailwind CSS (in `frontend/`)
- **Audio**: ffmpeg for PCM → G.711 ulaw transcoding; shairport-sync (optional) for RAOP
- **Docker**: multi-arch (amd64 + arm64), `network_mode: host` required for AirPlay mDNS

## Project Layout

- `cmd/` — cobra CLI entry point (`root.go`, `serve.go`)
- `internal/api/` — Echo HTTP server + handlers
- `internal/airplay/` — RAOP receiver (pure-Go `server.go` + shairport-sync wrapper)
- `internal/cameras/` — camera client implementations (Hikvision, etc.) + registry
- `internal/config/` — config model + SQLite persistence
- `internal/db/` — SQLite schema + migrations
- `internal/mqtt/` — Frigate MQTT subscriber
- `internal/tts/` — TTS client (OpenAI-compatible)
- `frontend/src/` — Svelte components

## Release

Tag `vX.Y.Z` + push → GitHub Actions builds multi-arch Docker image → push to ghcr.io.

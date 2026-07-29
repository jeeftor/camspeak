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

- `cmd/` — cobra CLI entry point (`root.go`, `serve.go`, `speak.go`, etc.)
- `internal/api/` — Echo HTTP server + handlers (split by domain: `handlers.go`, `vision.go`, `library.go`, `audio.go`, `camera_config.go`, `tts_config.go`, `rules_airplay_config.go`)
- `internal/airplay/` — RAOP receiver (pure-Go: `server.go`, `rtsp.go`, `rtsp_handlers.go`, `session.go`, `audio.go`, `fairplay.go` + shairport-sync wrapper)
- `internal/cameras/` — camera client implementations (Hikvision, Reolink, go2rtc, ONVIF) + registry
- `internal/config/` — config model (`model.go`), loading (`load.go`), SQLite persistence (`db.go`)
- `internal/db/` — SQLite schema + migrations
- `internal/frigate/` — Frigate NVR camera discovery
- `internal/library/` — preset store (raw audio on disk, metadata in SQLite)
- `internal/mqtt/` — Frigate MQTT subscriber
- `internal/tts/` — TTS client (OpenAI-compatible)
- `internal/util/` — shared utilities (`digest.go`, `stream.go`, `audio.go`, `net.go`, `url.go`)
- `internal/vision/` — Vision LLM client
- `frontend/src/` — Svelte components

## Release

Tag `vX.Y.Z` + push → GitHub Actions builds multi-arch Docker image → push to ghcr.io.

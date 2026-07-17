# camspeak

Camera audio router — stream TTS and audio to IP camera speakers via Hikvision ISAPI, Reolink, go2rtc, or ONVIF RTSP backchannel.

## Build & Run

```bash
make help        # list all targets
make build       # build frontend + Go binary (with version ldflags)
make run         # run the server locally
make test        # run tests via gotestsum
make frontend    # build frontend only (bun)
make docker      # build multi-arch Docker image
make clean       # remove build artifacts
```

**Prerequisite**: `make build` requires the frontend to be built first (`make frontend`), since `main.go` embeds `frontend/dist/`. The `build` target depends on `frontend` automatically.

## Configuration

All configuration is stored in SQLite at `<data-dir>/camspeak.db`. No YAML files.

### Config priority
1. **Env vars** (always win) — `CAMSPEAK_*` prefix
2. **SQLite preferences** — stored in the `preferences`, `tts_presets`, `cameras`, `rules` tables
3. **Built-in defaults** — sensible values for local dev

### Data directory
- Default: `./data` (local dev) or `/config` (Docker, via `CAMSPEAK_DATA_DIR`)
- Contains: `camspeak.db` (SQLite) + `library/` (raw audio files)

### Camera discovery
Cameras are auto-discovered from Frigate NVR on startup if `CAMSPEAK_FRIGATE_URL` is set and no cameras exist in the database. Manual discovery:
```bash
camspeak discover              # uses CAMSPEAK_FRIGATE_URL from env/DB
camspeak discover --frigate http://frigate:5000  # explicit URL
```

### TTS presets
Multiple TTS endpoints can be configured (klipbord-style presets). The active preset is used at runtime. Switch via REST API or env vars (env overrides active preset).

### Key env vars
| Variable | Description | Default |
|---|---|---|
| `CAMSPEAK_DATA_DIR` | Data directory (DB + library) | `./data` |
| `CAMSPEAK_LOG_LEVEL` | Log level: debug, info, warn, error | `info` |
| `CAMSPEAK_PORT` | HTTP server port | `8585` |
| `CAMSPEAK_FRIGATE_URL` | Frigate NVR URL for auto-discovery | (none) |
| `CAMSPEAK_GO2RTC_URL` | go2rtc URL for cameras using `go2rtc` type | (none) |
| `CAMSPEAK_ADVERTISE_IP` | IP that go2rtc can reach camspeak on (for Docker) | (auto-detect) |
| `CAMSPEAK_TTS_URL` | TTS API endpoint (overrides active preset) | (from active preset) |
| `CAMSPEAK_TTS_MODEL` | TTS model name | (from active preset) |
| `CAMSPEAK_TTS_VOICE` | Default TTS voice | (from active preset) |
| `CAMSPEAK_MQTT_BROKER` | MQTT broker URL | (none — MQTT disabled) |
| `CAMSPEAK_MQTT_USER` / `CAMSPEAK_MQTT_PASS` | MQTT credentials | (none) |
| `CAMSPEAK_VISION_URL` | Vision LLM endpoint (OpenAI-compatible) | (none) |
| `CAMSPEAK_VISION_MODEL` | Vision model name | (none) |
| `CAMSPEAK_VISION_API_KEY` | Vision API key | (none) |
| `CAMSPEAK_VISION_PROMPT` | Global default vision prompt | (hardcoded default) |
| `CAM_<NAME>_IP` / `CAM_<NAME>_USER` / `CAM_<NAME>_PASS` | Per-camera credential overrides | (from DB) |

### .env file
Copy `.env.example` to `.env` for local dev. Loaded by godotenv at startup. Gitignored.

## Architecture

- `cmd/` — Cobra CLI commands (`serve`, `speak`, `beep`, `list`, `discover`)
- `internal/api/` — Echo HTTP server, REST handlers, MCP endpoint, SSE events, config API
- `internal/cameras/` — Camera speaker clients (Hikvision ISAPI, Reolink, go2rtc stream-to-camera, ONVIF RTSP backchannel via gortsplib)
- `internal/config/` — SQLite-based config loading with env var overrides
- `internal/db/` — SQLite database initialization (modernc.org/sqlite, pure Go)
- `internal/frigate/` — Frigate NVR camera discovery (parses /config/raw)
- `internal/library/` — Preset store (raw audio on disk, metadata in SQLite)
- `internal/mqtt/` — Frigate MQTT subscriber for auto-speak rules
- `internal/tts/` — OpenAI-compatible TTS client (Kokoro)
- `frontend/` — Svelte 5 SPA (Vite, Bun)

### SQLite tables
- `presets` — preset metadata (name, category, text, voice, duration, raw_path)
- `events` — speak/play/beep event log for SSE history
- `preferences` — key-value runtime preferences (port, library path, frigate URL, MQTT)
- `tts_presets` — named TTS endpoint configurations (klipbord-style)
- `cameras` — camera definitions (name, type, ip, user, pass, channel, stream)

### Camera types
| Type | Protocol | Audio Method | Requirements |
|---|---|---|---|
| `hikvision` | ISAPI Two-Way Audio | HTTP PUT to `/ISAPI/Streaming/channels/{ch}/audioData` | Camera must support ISAPI (mainstream Hikvision) |
| `reolink` | Reolink HTTP API | Stub (not yet implemented) | — |
| `go2rtc` | go2rtc stream-to-camera API | `POST http://go2rtc:1984/api/streams?dst=<stream>&src=ffmpeg:<url>#audio=pcmu` | go2rtc must have a stream with `#backchannel=1`. Set `CAMSPEAK_GO2RTC_URL`. |
| `onvif` | ONVIF RTSP backchannel | Direct RTP/G.711 via gortsplib (no external deps) | Camera must advertise `a=sendonly` audio track in RTSP SDP |
- `rules` — MQTT-triggered auto-speak rules

### REST API
- `GET /api/config` — current runtime config
- `GET/PUT /api/config/vision` — vision endpoint config (URL, model, API key, default prompt)
- `GET/POST /api/config/tts` — list/create TTS presets
- `PUT/DELETE /api/config/tts/:name` — update/delete TTS preset
- `POST /api/config/tts/:name/activate` — set active TTS preset
- `GET/POST /api/config/cameras` — list/add cameras
- `DELETE /api/config/cameras/:name` — remove camera
- `GET/POST /api/config/rules` — list/create MQTT rules
- `GET /api/health` — health check with version
- `GET /api/events` — SSE event stream
- `POST /api/speak` — TTS to camera
- `POST /api/play` — preset to camera
- `POST /api/beep` — test tone
- `POST /api/broadcast` — broadcast to all cameras
- `GET/POST /api/library` — preset management
- `ANY /mcp` — MCP endpoint

## Docker

```yaml
services:
  camspeak:
    image: ghcr.io/jeeftor/camspeak:latest
    environment:
      CAMSPEAK_DATA_DIR: /config
      CAMSPEAK_FRIGATE_URL: http://frigate:5000
      CAMSPEAK_TTS_URL: http://tts:8080/v1/audio/speech
      CAMSPEAK_MQTT_BROKER: tcp://mqtt:1883
    volumes:
      - /data/config/camspeak:/config
    ports:
      - "8585:8585"
```

## Pre-commit hooks

Uses [prek](https://github.com/j178/prek) (or pre-commit) with `.pre-commit-config.yaml`:
- golangci-lint (linting + formatting)
- golines (line shortening, max 100 chars)
- gitleaks (secret detection)
- Standard file hygiene (trailing whitespace, EOF, YAML validation)

Install:
```bash
prek install
```

## Tech stack
- Go 1.26, Cobra, Echo, mcp-go
- SQLite via modernc.org/sqlite (pure Go, no CGO)
- Svelte 5 + Vite + Bun
- ffmpeg for audio transcoding (G.711ulaw 8kHz)

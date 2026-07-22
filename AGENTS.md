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
| `CAMSPEAK_AIRPLAY_ENABLED` | Enable AirPlay v1 (RAOP) receivers for all cameras | `false` |
| `CAMSPEAK_AIRPLAY_BASE_PORT` | Starting port for per-camera RAOP listeners | `5000` |
| `CAM_<NAME>_IP` / `CAM_<NAME>_USER` / `CAM_<NAME>_PASS` | Per-camera credential overrides | (from DB) |

### .env file
Copy `.env.example` to `.env` for local dev. Loaded by godotenv at startup. Gitignored.

## Architecture

- `cmd/` — Cobra CLI commands (`serve`, `speak`, `beep`, `list`, `discover`)
- `internal/api/` — Echo HTTP server, REST handlers, MCP endpoint, SSE events, config API
- `internal/airplay/` — RAOP (AirPlay v1) receiver: mDNS advertisement, RTSP server, RSA auth, AES decryption, ALAC decoding, audio pipeline to camera speakers
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
- `GET/PUT /api/config/airplay` — get/update AirPlay receiver config
- `GET /api/health` — health check with version
- `GET /api/openapi.json` — OpenAPI 3.0 spec
- `GET /swagger` — Swagger UI (interactive API explorer, CDN-hosted)
- `GET /api/events` — SSE event stream
- `POST /api/speak` — TTS to camera
- `POST /api/play` — preset to camera
- `POST /api/beep` — test tone
- `POST /api/stop` — stop audio on a camera (or all cameras if body empty)
- `POST /api/broadcast` — broadcast to all cameras
- `GET/POST /api/library` — preset management
- `PATCH /api/library/:category/:name` — rename preset
- `DELETE /api/library/:category/:name` — delete preset
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
- AirPlay v1 (RAOP) via grandcat/zeroconf (mDNS) + alicebob/alac (ALAC decoder), pure Go

## Debugging AirPlay

### Enable debug logging
```bash
CAMSPEAK_LOG_LEVEL=debug CAMSPEAK_AIRPLAY_ENABLED=true CAMSPEAK_AIRPLAY_BASE_PORT=5100 ./camspeak serve
```
Debug logs show every RTSP request/response, RTP packet seq/payload size, ALAC decode results, and audio chunk sends to camera.

### Verify mDNS advertisement
```bash
# Browse for RAOP services (should list each camera)
dns-sd -B _raop._tcp

# Check TXT records for a specific camera
dns-sd -L "0F6DF0EA0FEE@camera-name" _raop._tcp local
```
Expected TXT: `et=0,1 ek=1 cn=0,1 sr=44100 ss=16 tp=UDP am=camspeak`

### Capture RTSP/RTP traffic
```bash
# TCP (RTSP control channel) — port 5100
sudo tcpdump -i any -A 'tcp port 5100' -w airplay_rtsp.pcap

# UDP (RTP audio + timing + control)
sudo tcpdump -i any -A 'udp portrange 50000-65535' -w airplay_rtp.pcap

# All AirPlay traffic
sudo tcpdump -i any 'port 5100 or (udp and src <your-phone-ip>)' -w airplay.pcap

# Open in Wireshark
wireshark airplay.pcap
```

### Test RTSP handshake without iOS
Use the Python test script or curl:
```bash
# OPTIONS
echo -e "OPTIONS * RTSP/1.0\r\nCSeq: 1\r\n\r\n" | nc 127.0.0.1 5100

# Check if server is listening
lsof -i :5100
```

### Common issues
- **Port 5000 in use on macOS**: ControlCenter uses it. Use `CAMSPEAK_AIRPLAY_BASE_PORT=5100`
- **Camera not in AirPlay picker**: Check `dns-sd -B _raop._tcp` — if not listed, mDNS registration failed
- **iOS connects but no audio**: Check debug logs for `audio: RTP packet` lines — if none, UDP ports may be blocked by firewall
- **ALAC decode returns empty**: The `fmtp` parameters in SDP may not match what the ALAC decoder expects
- **ffmpeg not found**: AirPlay audio pipeline requires ffmpeg in PATH

## Logging

- Use `github.com/jeeftor/camspeak/internal/logging` for all new loggers:
  ```go
  var log = logging.New("mypackage", clog.InfoLevel)
  ```
- Do **not** call package-level `clog.Info/Error/Warn/Debug` directly; always use a logger from `logging.New` so timestamps, colored prefixes, and caller reporting are consistent.
- `CAMSPEAK_LOG_LEVEL` (debug, info, warn, error) is read by `cmd` and propagated to `api`, `cameras`, `mqtt`, `airplay`, `tts`, and `vision` at startup.
- Caller reporting (`file:line`) is enabled automatically when the level is `debug`.
- **Request IDs**: all API handlers receive an `X-Request-ID` and should log with the per-request logger from `h.logger(c)`. Internal helpers that run outside a request (e.g. `SpeakForMQTT`) use the package logger.
- **Error boundary**: synchronous HTTP handlers are the boundary for `Error` logs; lower-level clients should wrap errors and log them at `Debug`. This avoids double-logging the same failure.
- **Sanitize before logging**:
  - Strip embedded credentials from user-supplied URLs with `redactURL` (see `internal/api/util.go`).
  - Redact AirPlay SDP `fpaeskey`, `rsaaeskey`, and `aesiv` values before logging (`internal/airplay/server.go`).
  - Never log decrypted AES keys, API keys, or camera passwords.
- **Config redaction**: `GET /api/config` returns a sanitized copy (`Config.Sanitized()`) with `TTS.APIKey`, `Vision.APIKey`, `MQTT.Pass`, and camera passwords removed.

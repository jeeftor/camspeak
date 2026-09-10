<p align="center">
  <img src="assets/camspeak-lockup.svg" alt="camspeak — camera audio router" width="320">
</p>

<p align="center">
  Route text-to-speech and audio to IP camera speakers via Hikvision ISAPI, Reolink/go2rtc, or ONVIF RTSP backchannel.
</p>

[![CI](https://github.com/jeeftor/camspeak/actions/workflows/ci.yml/badge.svg)](https://github.com/jeeftor/camspeak/actions/workflows/ci.yml)
[![GHCR](https://img.shields.io/badge/ghcr.io-jeeftor%2Fcamspeak-blue)](https://github.com/jeeftor/camspeak/pkgs/container/camspeak)
![Go](https://img.shields.io/badge/Go-1.26-00ADD8)
![Platform](https://img.shields.io/badge/platform-linux%2Famd64%20%7C%20linux%2Farm64-lightgrey)
[![Security](https://github.com/jeeftor/camspeak/actions/workflows/security.yml/badge.svg)](https://github.com/jeeftor/camspeak/actions/workflows/security.yml)
[![Release](https://img.shields.io/github/v/release/jeeftor/camspeak)](https://github.com/jeeftor/camspeak/releases)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/github.com/jeeftor/camspeak/badge)](https://securityscorecards.dev/viewer/?uri=github.com/jeeftor/camspeak)

## Overview

**camspeak** is a self-hosted camera audio router. It takes text (or an audio
file), synthesizes speech via any OpenAI-compatible TTS endpoint (such as
OpenAI, Ollama, LiteLLM, or [Lemonade](https://github.com/lemonade-sdk/lemonade)
running [Kokoro](https://github.com/kokoro-ts/kokoro) locally), transcodes the
result to G.711ulaw 8kHz, and streams it to a camera speaker over Hikvision
ISAPI, Reolink/go2rtc, or ONVIF RTSP backchannel.

It is built to live alongside a [Frigate](https://frigate.video) NVR deployment:
cameras can be auto-discovered from Frigate's config and go2rtc can be used for
cameras that need a two-way audio bridge (especially Reolink doorbells).

A built-in AirPlay v1 receiver also lets each supported camera appear as a separate
AirPlay target on your iPhone, so you can stream music, calls, or any iOS audio
directly to a camera speaker.

A first-class [Home Assistant](https://www.home-assistant.io) integration is
available via [HACS](https://hacs.xyz) — see
[camspeak-hacs](https://github.com/jeeftor/camspeak-hacs) for media player
entities, real-time playback sensors, and smart services with entity selectors
and response data.

## Features

### Camera support

- **Hikvision ISAPI** — native two-way audio over `/ISAPI/Streaming/channels/{ch}/audioData`.
- **Reolink / go2rtc** — Reolink cameras (including doorbells) are routed through a
  [go2rtc](https://github.com/AlexxIT/go2rtc) stream using the ONVIF backchannel.
- **ONVIF RTSP backchannel** — direct RTP/G.711 to cameras that advertise an
  `a=sendonly` audio track, no external bridge required.
- **Per-camera gain** — a gain slider on each camera card controls volume for
  TTS, presets, URL playback, live streams, and AirPlay.

### Frigate & go2rtc integration

- **Frigate auto-discovery** — pull camera names, IPs, and stream definitions from
  Frigate's `/api/config` endpoint on first boot.
- **go2rtc detection** — auto-detect a reachable go2rtc instance from `CAMSPEAK_GO2RTC_URL`,
  Frigate's built-in go2rtc, or common local endpoints.
- **Camera type detection** — probe an IP to classify it as Hikvision, Reolink,
  or ONVIF when adding cameras manually.

### TTS & audio sources

- **OpenAI-compatible TTS** — works with OpenAI, Ollama, LiteLLM, or local Lemonade.
- **TTS presets** — klipbord-style named endpoint configurations with an active
  preset selected at runtime.
- **Audio library** — generate TTS clips, upload audio files, and save them as
  reusable presets organized by category.
- **Live audio streaming** — stream live URLs or `.pls`/`.m3u` playlists
  (e.g. LiveATC, internet radio) directly to a Hikvision camera speaker.
  Continuous streams and AirPlay are currently unsupported by the
  go2rtc, Reolink, and ONVIF backends; finite audio playback remains available.
- **Broadcast** — send TTS or a preset to all enabled cameras at once.

### Vision (describe & announce)

- **Describe** — `POST /api/describe` captures a still image from a camera, sends
  it to a vision LLM (any OpenAI-compatible chat completions endpoint — OpenAI,
  Ollama, LiteLLM, etc.), gets a text description back, then TTS-speaks that
  description on the same camera's speaker.
- **Cross-camera announce** — `POST /api/announce` captures an image from one
  camera, describes it with the vision model, and speaks the description on a
  *different* target camera's speaker. Useful for doorbells or cameras without
  speakers.
- **Vision config** — `GET/PUT /api/config/vision` manages the vision endpoint
  (URL, model, API key, default prompt). Configure via env vars
  (`CAMSPEAK_VISION_URL`, `CAMSPEAK_VISION_MODEL`, `CAMSPEAK_VISION_API_KEY`,
  `CAMSPEAK_VISION_PROMPT`) or the UI.
- **Vision Playground** — the frontend's Vision Playground lets you capture a
  frame from any camera (or upload your own image), run it against the
  configured vision model with a custom prompt, and view the description with
  timing breakdowns. You can also test a single image against *all* available
  vision models at once via a streaming endpoint.
- **Capture Benchmark** — benchmarks every capture method for a single camera,
  comparing capture time, image resolution, and file size. Enable "with vision"
  to also measure the full pipeline (capture + model inference).
- **Full Matrix Benchmark** — runs the full vision pipeline across every
  selected camera, prompt, capture method, and model in a streaming matrix,
  producing a side-by-side comparison of results.

### Automation & control

- **MCP endpoint** — expose `speak`, `play_preset`, `broadcast`, `list_cameras`,
  `list_presets`, `generate_preset`, and `beep` tools over the Model Context
  Protocol for LLM-driven automation (Claude, Cursor, OpenAI, Ollama via LiteLLM, etc.).
- **REST API** — full HTTP API for speaking, playing presets, broadcasting,
  managing cameras, TTS presets, and the audio library.
- **Home Assistant** — first-class HACS integration with media player entities, binary sensors, real-time SSE playback updates, and smart services with entity selectors, voice dropdowns, and response data. See [camspeak-hacs](https://github.com/jeeftor/camspeak-hacs). Also works via `rest_command` for simpler setups.
- **Svelte UI** — embedded Svelte 5 SPA served from the binary for point-and-click control.

### AirPlay receiver

- **AirPlay v1 target per supported camera** — each enabled Hikvision receiver shows up as a separate AirPlay
  speaker in the iOS picker.
- **iOS audio to camera** — stream music, calls, or any iOS audio to a camera
  speaker with per-camera gain, custom display name, and device icon model.
- **shairport-sync on Linux/Docker** — on Linux (including the Docker image),
  camspeak prefers [shairport-sync](https://github.com/mikebrady/shairport-sync),
  an external binary that handles the RAOP/FairPlay/ALAC work internally and
  emits decoded PCM on stdout. That PCM is transcoded by `ffmpeg` to G.711 µ-law
  8 kHz mono and streamed to the camera. The Docker image bundles
  shairport-sync, so no extra setup is required.
- **Built-in pure-Go RAOP on macOS** — on macOS, camspeak uses a built-in pure-Go
  RAOP receiver (no CGO) as a fallback when shairport-sync is not available.
  This path is compatible with iOS 18+ and iOS 26.

### Deployment

- **SQLite configuration** — no YAML. All preferences, cameras, and TTS presets
  live in a single `camspeak.db` file.
- **Multi-arch Docker** — `linux/amd64` and `linux/arm64` images published to GHCR.
- **Pure Go** — SQLite via `modernc.org/sqlite`, no CGO required.

## Camera dashboard

Camera cards keep **Speak**, **Presets**, and **Streams** directly on the main
screen. Type a message and speak, select an audio preset, or choose a saved stream
or paste its URL. **Files**, **Describe**, and **Preview** remain on each card.
Voice options, local audio preview, and custom repeat counts stay beside the
relevant action; zero additional repeats plays once and -1 repeats until stopped.
Your drafts stay available while you switch modes during the current dashboard
session. Volume and supported playback controls remain available on the card.
On phones, **More…** exposes the remaining pages and automation tools without
horizontal navigation scrolling.

Use **Broadcast** beside the Cameras heading for announcements to all enabled
cameras. Use **Arrange** to expose camera ordering controls. The **Diagnostics**
page contains the vision playground, capture benchmark, and full matrix benchmark;
connection settings remain under **Config**.

## Quick start with Docker

Pull the image:

```bash
docker pull ghcr.io/jeeftor/camspeak:latest
```

Create a `docker-compose.yml`:

```yaml
services:
  camspeak:
    image: ghcr.io/jeeftor/camspeak:latest
    container_name: camspeak
    environment:
      - TZ=${TZ:-America/Denver}
      - CAMSPEAK_DATA_DIR=/config
      - CAMSPEAK_FRIGATE_URL=${CAMSPEAK_FRIGATE_URL:-}
      - CAMSPEAK_GO2RTC_URL=${CAMSPEAK_GO2RTC_URL:-}
      - CAMSPEAK_TTS_URL=${CAMSPEAK_TTS_URL:-}
      - CAMSPEAK_TTS_MODEL=${CAMSPEAK_TTS_MODEL:-kokoro}
      - CAMSPEAK_TTS_VOICE=${CAMSPEAK_TTS_VOICE:-af_sky}
      - CAMSPEAK_AIRPLAY_ENABLED=${CAMSPEAK_AIRPLAY_ENABLED:-true}
      - CAMSPEAK_AIRPLAY_BASE_PORT=${CAMSPEAK_AIRPLAY_BASE_PORT:-5100}
      - CAMSPEAK_AIRPLAY_MODEL=${CAMSPEAK_AIRPLAY_MODEL:-RealityDevice14,1}
    volumes:
      - ${CONFIG_DIR:-./config}/camspeak:/config
    network_mode: host   # required for AirPlay mDNS advertisement and UDP RTP
    restart: unless-stopped
```

Bring it up:

```bash
docker compose up -d
```

Open the UI at `http://localhost:8585`. The MCP endpoint is at
`http://localhost:8585/mcp`.

The container bundles `ffmpeg` (required for transcoding to G.711ulaw 8kHz) and
exposes a volume at `/config` for the SQLite database and audio library.

AirPlay requires `network_mode: host` in the compose file (already set above) so
mDNS advertisement and UDP RTP traffic reach your LAN. If you disable AirPlay, you
can remove `network_mode: host` and map port `8585` explicitly instead.

## Configuration

camspeak stores all configuration in SQLite at `<data-dir>/camspeak.db`. There
are no YAML files.

### Config priority

Configuration is resolved in the following order (highest precedence first):

1. **Environment variables** — `CAMSPEAK_*` prefix, always win.
2. **SQLite preferences** — stored in the `preferences`, `tts_presets`,
   and `cameras` tables.
3. **Built-in defaults** — sensible values for local development.

A `.env` file (gitignored) is loaded by godotenv at startup for local dev. Copy
`.env.example` to `.env` to get started.

### Data directory

| Setting | Local dev | Docker |
|---|---|---|
| Default | `./data` | `/config` (via `CAMSPEAK_DATA_DIR`) |
| Contents | `camspeak.db` + `library/` | `camspeak.db` + `library/` |

### Key environment variables

| Variable | Description | Default |
|---|---|---|
| `CAMSPEAK_DATA_DIR` | Data directory (DB + library) | `./data` |
| `CAMSPEAK_PORT` | HTTP server port | `8585` |
| `CAMSPEAK_CORS_ORIGIN` | Comma-separated exact browser origins allowed in addition to the server's own origin; include scheme and port | (same-origin only) |
| `CAMSPEAK_FRIGATE_URL` | Frigate NVR URL for auto-discovery | (none) |
| `CAMSPEAK_TTS_URL` | TTS API endpoint (overrides active preset) | (from active preset) |
| `CAMSPEAK_TTS_MODEL` | TTS model name | (from active preset) |
| `CAMSPEAK_TTS_VOICE` | Default TTS voice | (from active preset) |
| `CAMSPEAK_TTS_API_KEY` | TTS API key (cloud providers only) | (none) |
| `CAMSPEAK_VISION_URL` | Vision LLM endpoint (OpenAI-compatible chat completions, e.g. OpenAI, Ollama, LiteLLM) | (none) |
| `CAMSPEAK_VISION_MODEL` | Vision model name | (none) |
| `CAMSPEAK_VISION_API_KEY` | Vision API key (cloud providers only) | (none) |
| `CAMSPEAK_VISION_PROMPT` | Global default vision prompt (fallback for Describe/Vision) | (hardcoded default) |
| `CAMSPEAK_AIRPLAY_ENABLED` | Enable AirPlay v1 receivers for all cameras | `false` |
| `CAMSPEAK_AIRPLAY_BASE_PORT` | Starting port for per-camera RAOP listeners | `5100` |
| `CAMSPEAK_AIRPLAY_GAIN` | Default AirPlay digital gain when per-camera gain is unset | `1.0` |
| `CAMSPEAK_AIRPLAY_MODEL` | Default device model advertised over mDNS (controls the iOS AirPlay icon) | `RealityDevice14,1` |
| `CAMSPEAK_PRIME_SILENCE_MS` | Ms of silence prepended before audio to warm the camera speaker (prevents garbled start). 0 disables. | `150` |
| `CAM_<NAME>_IP` | Override IP for a discovered camera | (from DB) |
| `CAM_<NAME>_USER` | Override username for a discovered camera | (from DB) |
| `CAM_<NAME>_PASS` | Override password for a discovered camera | (from DB) |

Per-camera env vars use the camera name uppercased with hyphens replaced by
underscores. For example, a camera named `back-yard` maps to
`CAM_BACK_YARD_IP`, `CAM_BACK_YARD_USER`, and `CAM_BACK_YARD_PASS`.

### Camera discovery

Cameras are auto-discovered from a Frigate NVR instance on startup when
`CAMSPEAK_FRIGATE_URL` is set and no cameras exist in the database yet. The
discoverer queries Frigate's `/api/config` endpoint, parses the `go2rtc` stream
definitions for RTSP URLs, extracts the camera IP, classifies the vendor
(Hikvision, Reolink, or ONVIF), and deduplicates by IP (preferring main-stream
entries over sub-streams).

When adding a camera manually in the UI, click **Detect** to probe the IP and
auto-fill the camera type. For Reolink cameras (including doorbells), camspeak
routes audio through a go2rtc stream with an ONVIF backchannel source; set
`CAMSPEAK_GO2RTC_URL` and add the matching stream in go2rtc.

Credentials returned by the Frigate API are often masked as `*:*`. Set real
credentials via the `CAM_<NAME>_USER` and `CAM_<NAME>_PASS` environment
variables.

Manual discovery:

```bash
camspeak discover                              # uses CAMSPEAK_FRIGATE_URL
camspeak discover --frigate http://frigate:5000   # explicit URL
```

### TTS presets

Multiple TTS endpoints can be configured in a klipbord-style preset system. The
**active** preset is used at runtime for all TTS requests. Any OpenAI-compatible
TTS provider works — for example:

| Preset | Endpoint | Model | Voice | Description |
|---|---|---|---|---|
| `openai` | `https://api.openai.com/v1/audio/speech` | `tts-1` | `alloy` | OpenAI cloud (requires API key) |
| `lemonade` | `http://localhost:13305/v1/audio/speech` | `kokoro` | `af_sky` | Local Lemonade (GPU) serving Kokoro |
| `ollama` | `http://localhost:11434/v1/audio/speech` | — | — | Local Ollama endpoint (if TTS model loaded) |
| `litellm` | `http://localhost:4000/v1/audio/speech` | `kokoro` | `af_sky` | LiteLLM proxy to any supported provider |

Two defaults (`lemonade` and `openai`) are seeded on first boot. Switch the
active preset via the REST API or env vars. `CAMSPEAK_TTS_URL`,
`CAMSPEAK_TTS_MODEL`, and `CAMSPEAK_TTS_VOICE` override the active preset at
runtime without changing the database.

## CLI commands

camspeak is a Cobra CLI. All commands accept `--data-dir` to override the data
directory (default: `./data` or `CAMSPEAK_DATA_DIR`).

| Command | Description | Example |
|---|---|---|
| `serve` | Start the HTTP server (default command) | `camspeak serve` |
| `discover` | Discover cameras from Frigate and save to DB | `camspeak discover --frigate http://frigate:5000` |
| `speak <camera> <text>` | Send TTS to a camera speaker | `camspeak speak backyard "Hello there" -v af_sky` |
| `beep <camera>` | Play an 800Hz test beep on a camera | `camspeak beep frontdoor` |
| `list <cameras\|presets>` | List configured cameras or saved presets | `camspeak list cameras` |

Examples:

```bash
# Run the server
camspeak serve

# Speak to a camera with a specific voice
camspeak speak backyard "Person detected at the front door" -v am_michael

# List cameras with online status
camspeak list cameras

# List saved audio presets
camspeak list presets
```

## REST API

All routes are under `/api`. The server listens on port `8585` by default.

Browser requests are restricted to the server's own origin by default. To serve
your frontend separately, set `CAMSPEAK_CORS_ORIGIN` to its exact origin, for
example `http://localhost:5173`. Separate multiple origins with commas; wildcards
are not supported. Clients without an `Origin` header, including Home Assistant
and command-line tools, continue to work. This policy does not authenticate API
clients; keep the service on a trusted network or behind your authenticated proxy.

Audio requests accept an optional `gain` from `0` to `10`. Omit it to use the
target camera's saved gain; explicitly send `0` to mute. For broadcasts, omission
preserves each camera's own setting. Identify library presets using both
`category` and `preset`, for example
`{"camera":"backyard","category":"alerts","preset":"doorbell"}`. Name-only
requests are supported when the name is unique; duplicate names require a category.

TTS and vision configuration responses omit API keys and return `has_api_key`.
When editing settings, an omitted or empty `api_key` preserves the saved key.
To remove it, send `clear_api_key: true` with an empty key. Environment variables
still override saved settings, including after activating a different TTS preset.

Uploads return HTTP `202` with a `job_id`. Poll the job endpoint until `status`
is `done`, then use the returned preset's category and name for playback. Handle
`error` as a failed upload. The entire multipart request is limited to 64 MiB
(HTTP `413` when exceeded), and two uploads/transcodes can run at once (HTTP `503`
when capacity is occupied). Retry after an active upload finishes.

### Audio

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/speak` | Send TTS to a named camera |
| `POST` | `/api/play` | Play a saved preset on a camera (supports `loop` integer: -1 = infinite, 0 = no loop, N = play N+1 times; looped presets are pausable/resumable via `/api/pause` and `/api/resume`) |
| `POST` | `/api/play-url` | Download an audio URL and play it on a camera |
| `POST` | `/api/play-stream` | Stream a live URL or `.pls`/`.m3u` playlist to a camera (Hikvision, requires ffmpeg) |
| `POST` | `/api/beep` | Play an 800Hz test beep on a camera |
| `POST` | `/api/stop` | Stop audio, live streams, and reset AirPlay for a camera (or all cameras if body empty) |
| `POST` | `/api/pause` | Pause a live `/api/play-stream` stream (camera or all) — suspends ffmpeg via SIGSTOP without tearing down the camera connection |
| `POST` | `/api/resume` | Resume a paused stream (camera or all) via SIGCONT |
| `GET` | `/api/playback` | Current playback state for all enabled cameras (preparing/playing/paused/idle, `can_pause`, source, detail, timestamps) |
| `POST` | `/api/broadcast` | Broadcast TTS or a preset to all cameras |
| `GET` | `/api/cameras` | List cameras with online status |
| `GET` | `/api/cameras/:name/info` | Query camera device info & streaming settings (ISAPI/ONVIF, read-only) |
| `PUT` | `/api/cameras/:name/volume` | Set runtime gain (0-10); takes effect on next audio chunk without restarting playback; also persists to config |
| `GET` | `/api/voices` | List available TTS voices |

### Library

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/library` | List all saved presets |
| `POST` | `/api/library` | Generate a TTS clip and save as a preset |
| `POST` | `/api/library/upload` | Upload an audio file (async — returns `job_id` for progress polling) |
| `GET` | `/api/library/upload/jobs/:id` | Poll upload/transcode job progress (percent, step, error) |
| `DELETE` | `/api/library/:category/:name` | Delete a preset |
| `PATCH` | `/api/library/:category/:name` | Rename a preset |
| `GET` | `/api/library/:category/:name/preview` | Preview a preset's audio |

### Vision

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/describe` | Capture an image from a camera, send to the vision LLM, get a description, then TTS-speak it on the same camera |
| `POST` | `/api/announce` | Capture an image from one camera, describe it, and speak the description on a target camera |
| `GET` | `/api/config/vision` | Get vision endpoint config (URL, model, API key, default prompt) |
| `PUT` | `/api/config/vision` | Update vision endpoint config |

### Config

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/config` | Current runtime configuration |
| `GET` | `/api/config/vision` | Get vision endpoint config (URL, model, API key, default prompt) |
| `PUT` | `/api/config/vision` | Update vision endpoint config |
| `GET` | `/api/config/tts` | List all TTS presets |
| `POST` | `/api/config/tts` | Create a TTS preset |
| `PUT` | `/api/config/tts/:name` | Update a TTS preset |
| `DELETE` | `/api/config/tts/:name` | Delete a TTS preset (not the active one) |
| `POST` | `/api/config/tts/:name/activate` | Set a TTS preset as active |
| `GET` | `/api/config/cameras` | List configured cameras |
| `POST` | `/api/config/cameras` | Add or update a camera; includes per-camera `gain` (default `3.0`) |
| `POST` | `/api/config/cameras/detect` | Probe a camera IP and auto-detect vendor type (Hikvision, Reolink, ONVIF) |
| `POST` | `/api/config/cameras/discover` | Discover cameras from a Frigate NVR instance |
| `DELETE` | `/api/config/cameras/:name` | Remove a camera |
| `GET` | `/api/config/airplay` | Get AirPlay receiver configuration |
| `PUT` | `/api/config/airplay` | Update AirPlay receiver configuration |

### System

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/health` | Health check with version |
| `GET` | `/api/openapi.json` | OpenAPI 3.0 spec |
| `GET` | `/swagger` | Swagger UI (interactive API explorer) |
| `GET` | `/api/events` | Server-sent events stream (speak/play/beep log) |
| `ANY` | `/mcp` | MCP endpoint (streamable HTTP transport) |

## MCP tools

camspeak exposes an MCP server at `POST /mcp` using the streamable HTTP
transport, allowing any LLM agent or client — Claude, Cursor, OpenAI, Ollama
via LiteLLM, etc. — to control camera audio.

| Tool | Description | Required params |
|---|---|---|
| `speak` | Send text-to-speech audio to a named camera speaker | `camera`, `text` |
| `play_preset` | Play a saved audio preset on a camera speaker | `camera`, `preset` |
| `broadcast` | Send TTS or a preset to all cameras simultaneously | `text` or `preset` |
| `list_cameras` | List all configured cameras and their online status | (none) |
| `list_presets` | List all saved audio presets in the library | (none) |
| `generate_preset` | Generate a TTS audio clip and save it as a reusable preset | `name`, `text` |
| `beep` | Play an 800Hz test beep on a camera | `camera` |

Optional parameters: `voice` (TTS voice) and `category` (preset category) where
applicable.

## Home Assistant

camspeak has a first-class [Home Assistant](https://www.home-assistant.io)
integration available via [HACS](https://hacs.xyz). The integration provides:

- **Media Player entities** for each camera — play presets, pause/resume/stop, volume control
- **Binary sensors** — camera online/offline status
- **Playback sensors** — real-time state via SSE (no polling lag)
- **Smart services** with entity selectors, voice dropdowns, gain sliders, and response data:
  - `camspeak.speak` — TTS with voice selector, returns timing data
  - `camspeak.play_preset` — play from library, supports loop
  - `camspeak.play_stream` / `camspeak.play_url` — live streams or audio files
  - `camspeak.broadcast` — TTS or preset to all cameras
  - `camspeak.beep` / `camspeak.stop` / `camspeak.pause` / `camspeak.resume`
- **Zeroconf discovery** — camspeak servers are auto-discovered on your network

### Install the HACS integration

1. In HACS, add `https://github.com/jeeftor/camspeak-hacs` as a custom repository (type: Integration)
2. Install "Camspeak" and restart Home Assistant
3. Go to **Settings → Devices & Services → Add Integration** and search for "Camspeak"
4. Enter your camspeak server URL

See the [camspeak-hacs README](https://github.com/jeeftor/camspeak-hacs) for full documentation.

### REST commands (alternative)

If you prefer not to install the HACS integration, camspeak also works via the
built-in `rest_command` platform. Define REST commands that call camspeak's
API, then trigger them from automations, dashboards, or webhooks.

### Setup

Add to `configuration.yaml` (replace `CAMSPEAK_IP` with your camspeak host):

```yaml
rest_command:
  camspeak_speak:
    url: http://CAMSPEAK_IP:8585/api/speak
    method: POST
    content_type: application/json
    payload: '{"camera":"{{ camera }}","text":"{{ text }}","voice":"{{ voice }}"}'

  camspeak_broadcast:
    url: http://CAMSPEAK_IP:8585/api/broadcast
    method: POST
    content_type: application/json
    payload: '{"text":"{{ text }}","voice":"{{ voice }}"}'

  camspeak_play_preset:
    url: http://CAMSPEAK_IP:8585/api/play
    method: POST
    content_type: application/json
    payload: '{"camera":"{{ camera }}","preset":"{{ preset }}"}'
```

### Example automation

```yaml
automation:
  - alias: "Backyard person detected"
    trigger:
      - platform: state
        entity_id: binary_sensor.backyard_person
        to: "on"
    condition:
      - condition: time
        after: "06:00:00"
        before: "22:00:00"
    action:
      - service: rest_command.camspeak_speak
        data:
          camera: backyard
          text: "Person detected in the backyard"
          voice: af_sky
```

This gives you HA's full condition/template engine (time windows, presence
detection, multi-sensor logic) for triggering announcements. For the full
integration experience (entity selectors, real-time state, response data),
use the HACS integration above.

See the **Home Assistant** section in the UI for copy-paste-ready snippets including
webhook triggers and dashboard buttons.

## Development

### Make targets

| Target | Description |
|---|---|
| `make help` | List all available targets |
| `make build` | Build the frontend and Go binary (with version ldflags) |
| `make run` | Run the server locally (`go run main.go serve`) |
| `make frontend` | Build the frontend assets (`cd frontend && bun install && bun run build`) |
| `make test` | Run the test suite via `gotestsum` |
| `make docker` | Build the multi-arch Docker image (`linux/amd64`, `linux/arm64`) |
| `make clean` | Remove build artifacts |

`make build` depends on `make frontend` automatically, since `main.go` embeds
`frontend/dist/`.

### Pre-commit hooks

Uses [prek](https://github.com/j178/prek) (or pre-commit) with
`.pre-commit-config.yaml`:

- **golangci-lint** — linting and formatting
- **golines** — line shortening (max 100 chars)
- **gitleaks** — secret detection
- Standard file hygiene (trailing whitespace, EOF, YAML validation)

Install:

```bash
prek install
```

### Tech stack

- **Go 1.26** — Cobra (CLI), Echo (HTTP), mcp-go (MCP), charmbracelet/log
- **SQLite** — `modernc.org/sqlite` (pure Go, no CGO)
- **Svelte 5 + Vite + Bun** — embedded SPA frontend
- **ffmpeg** — audio transcoding to G.711ulaw 8kHz (8000 bytes/sec)
- **Docker** — multi-stage build, Alpine runtime, multi-arch via buildx

## Architecture

### Internal packages

| Package | Responsibility |
|---|---|
| `cmd/` | Cobra CLI commands (`serve`, `discover`, `speak`, `beep`, `list`) |
| `internal/api/` | Echo HTTP server, REST handlers, MCP endpoint, SSE events, config API |
| `internal/cameras/` | Camera speaker clients — Hikvision ISAPI, Reolink/go2rtc, ONVIF RTSP backchannel |
| `internal/config/` | SQLite-based config loading with env var overrides |
| `internal/db/` | SQLite database initialization and schema |
| `internal/frigate/` | Frigate NVR camera discovery (parses `/api/config` and go2rtc streams) |
| `internal/library/` | Preset store — raw audio on disk, metadata in SQLite |
| `internal/tts/` | OpenAI-compatible TTS client (works with OpenAI, Ollama, LiteLLM, Lemonade, etc.) |
| `frontend/` | Svelte 5 SPA (Vite, Bun), embedded into the Go binary |

### SQLite tables

| Table | Contents |
|---|---|
| `presets` | Preset metadata (name, category, text, voice, duration, raw_path) |
| `events` | Speak/play/beep event log for SSE history |
| `preferences` | Key-value runtime preferences (port, library path, frigate URL) |
| `tts_presets` | Named TTS endpoint configurations (klipbord-style, one active) |
| `cameras` | Camera definitions (name, type, ip, user, pass, channel, gain, airplay_name, airplay_model, airplay_enabled) |

### Audio pipeline

1. Text is sent to any OpenAI-compatible TTS endpoint (`/v1/audio/speech`).
2. The returned WAV is transcoded by `ffmpeg` to raw G.711ulaw 8kHz.
3. The raw audio is saved to the library on disk and metadata to SQLite.
4. The camera client opens an ISAPI two-way audio session (digest auth), then
   streams the raw file throttled to 8000 bytes/sec to match real-time playback.

For **AirPlay**, iPhone audio is decoded by the built-in RAOP receiver or
`shairport-sync`, transcoded by `ffmpeg`, and streamed to the camera in real time.
For **live streams** (`/api/play-stream`), the URL is resolved if it is a `.pls`
or `.m3u` playlist, then `ffmpeg` decodes the live stream to G.711ulaw and pipes
it directly to the camera's streaming implementation. Continuous playback is
supported by Hikvision; go2rtc, Reolink, and ONVIF reject continuous streams
instead of buffering indefinitely. Check `/api/cameras` capabilities before
offering live-stream or AirPlay controls.

A per-camera **gain** value (default `3.0`) controls the digital volume of TTS,
presets, URL playback, live streams, and AirPlay. Setting it to `0` produces
G.711 silence. Per-request gain overrides are optional; omission preserves the
camera setting.

### Hikvision ISAPI flow

The Hikvision client (`internal/cameras/hikvision.go`) uses digest
authentication and the ISAPI `TwoWayAudio` endpoint:

1. `PUT /close` — clear any stale session.
2. `PUT /open` — open a two-way audio session, receive a `sessionId`.
3. `PUT /audioData?sessionId=...` — stream G.711ulaw audio, rate-limited to
   8000 bytes/sec via a throttled reader.
4. `PUT /close?sessionId=...` — close the session.

# Changelog

All notable changes to camspeak are documented here.
Versions follow [Semantic Versioning](https://semver.org/).

---

## [v2.10.0] — 2026-08-11

### Added
- **Per-camera vision stream selection** — cameras can now be configured to grab vision snapshots from any go2rtc stream instead of Frigate's detect stream. Set `vision_stream` (e.g. `frontyard_sub`) and optionally `vision_width` (e.g. 1280) in the camera settings. This allows using lower-resolution substreams for vision, dramatically reducing image size and vision model latency (e.g. frontyard was sending 4K/1.9MB images; with a substream at 1280px it's ~9KB).
- **`GET /api/streams`** — lists all available go2rtc streams with video codec and active status. Used by the camera settings UI to populate the stream dropdown.
- **`GET /api/snapshot/:camera?stream=<name>&width=<px>`** — snapshot endpoint now accepts optional `stream` and `width` query params. When `stream` is specified, grabs a frame from that go2rtc stream via ffmpeg (with optional scaling) instead of Frigate.
- **ffmpeg-based frame capture** — `grabFrameFromStream()` uses ffmpeg to capture a single JPEG frame from any go2rtc RTSP stream, with optional width scaling.
- Camera settings UI now has a "Vision Stream" section with a dropdown of available go2rtc streams and a max-width input.

### Changed
- The describe/vision flow now uses `fetchSnapshot()` which checks the camera's `vision_stream` config first, falling back to Frigate's detect stream if not set.
- `CameraConfig` struct has new `VisionStream` and `VisionWidth` fields, stored in SQLite (`vision_stream`, `vision_width` columns).

---

## [v2.9.0] — 2026-08-10

### Added
- **Time to first sound (TTFS) metric** — the timing display now shows `⏱ X.Xs` as the headline, representing the latency from request start to the first audio byte reaching the camera. This is the number that matters for responsiveness — like "time to first token" in LLM chat.
- **Split send timing into `send_open_ms` (latency) and `send_playback_ms` (duration)** — the old `send_ms` lumped together the connection setup and the real-time audio streaming. Now `send_open_ms` measures channel open + auth + first chunk write (latency), and `send_playback_ms` measures the throttled streaming at 8000 bytes/sec (playback duration, not latency).
- `ttfs_ms` field added to all API responses (`/api/speak`, `/api/play`, `/api/describe`, `/api/vision/test`).
- Tooltip now shows "Time to first sound" and "Total (incl. playback)" separately, with per-step descriptions.

### Changed
- `Speaker.SendRaw` interface changed from `error` to `(SendTiming, error)` where `SendTiming` has `OpenMs` and `PlaybackMs` fields. All camera implementations (Hikvision, go2rtc, ONVIF, Reolink) and callers updated.
- `formatTimingSummary` now excludes `send_playback_ms` from the breakdown (it's not latency) and uses `ttfs_ms` as the headline instead of `total_ms`.

---

## [v2.8.4] — 2026-08-10

### Fixed
- **AirPlay badge not showing on camera cards** — `GET /api/cameras` was not returning `airplay_enabled`, `airplay_name`, or `airplay_model` fields, so the frontend had no data to show the indicator. Added the missing fields to the handler response.

---

## [v2.8.2] — 2026-08-10

### Added
- **AirPlay status indicator on camera cards** — a violet AirPlay icon badge now appears in each camera card header when AirPlay is enabled for that camera, with a tooltip showing the receiver name. Makes it easy to see at a glance which cameras have AirPlay active.

### Fixed
- Fixed broken frontend build — the lucide-svelte icon is exported as `Airplay` (lowercase p), not `AirPlay`.

---

## [v2.8.1] — 2026-08-10

### Fixed
- **AirPlay toggle 405 Method Not Allowed** — the frontend called `POST /api/config/airplay/:camera/toggle` but the backend registers that route as `PATCH`. Fixed `frontend/src/lib/api.ts` to use `PATCH`, matching `internal/api/server.go`.

### Added
- CODEOWNERS, OpenSSF Scorecard config, and Scorecard badge.

---

## [v2.7.0] — 2026-07-29

### Per-step timing tracking across all actions

Every audio and vision action now returns a timing breakdown so you can see exactly where time is spent — TTS generation, transcoding, snapshot fetch, vision model inference, and camera send.

### Added
- **`internal/api/timing.go`** — `StepTimings` struct for tracking per-step durations. `Add(name, start)` records elapsed time; `Ms()` returns a map for JSON responses.
- Timing data in all API responses:
  - `POST /api/speak` → `timings: {tts_ms, transcode_ms, send_ms}`
  - `POST /api/play` → `timings: {load_ms, transcode_ms, send_ms}`
  - `POST /api/describe` → `timings: {snapshot_ms, vision_ms, tts_ms, transcode_ms, send_ms}`
  - `POST /api/vision/test` → `timings: {snapshot_ms, vision_ms}`
  - `POST /api/tts/preview` → `X-TTS-Ms` response header
  - `POST /api/library` → `timings: {tts_ms, save_ms}`
- Frontend timing display:
  - CameraCard: `✓ sent (1.4s: TTS 450ms · transcode 120ms · send 800ms)`
  - VisionTest: `⏱ snap 300ms · vision 2.0s` below description
  - Library: `✓ Playing… (450ms)` and `✓ Saved (460ms: TTS 450ms)`
- `frontend/src/lib/utils.ts`: `formatMs`, `formatTimings`, `formatTimingSummary` helpers
- `frontend/src/lib/types.ts`: `Timings` interface + response types

### Changed
- `speakText` and `playPreset` now return `(*StepTimings, error)` instead of just `error`. All callers updated (Speak, Play, Describe, Broadcast, SpeakForMQTT, MCP tools).

---

## [v2.6.2] — 2026-07-29

### Migrated to official MCP Go SDK

Switched from the community `mark3labs/mcp-go` (v0.56.0, spec 2025-11-25) to the official `modelcontextprotocol/go-sdk` (v1.7.0, spec 2026-07-28). The official SDK is a Tier 1 implementation maintained with Google, passes the MCP conformance suite, and supports the new stateless protocol.

### Changed
- **MCP tool definitions now use typed Go structs** with `jsonschema` tags instead of stringly-typed `GetString` calls. Tool schemas are generated from the struct — typos in field names are now compile errors. All 7 tools migrated (speak, play_preset, broadcast, list_cameras, list_presets, generate_preset, beep).
- `mcp.AddTool` generic function replaces `s.AddTool(mcp.NewTool(...))` pattern.
- Error results use `CallToolResult{IsError: true, Content: ...}` instead of `mcp.NewToolResultError()`.
- `mcp.NewStreamableHTTPHandler` replaces `server.NewStreamableHTTPServer` — implements `http.Handler` so `echo.WrapHandler` works as before.
- `mark3labs/mcp-go` dependency removed entirely.

---

## [v2.6.1] — 2026-07-29

### Centralized API client + shared TypeScript types

### Added
- **`frontend/src/lib/api.ts`** — typed API client wrapping all 51 fetch calls. Single place for auth headers, error handling, and request formatting. Exports `apiClient` with methods for every endpoint.
- **`frontend/src/lib/types.ts`** — TypeScript interfaces matching Go backend structs (Camera, TTSPreset, Rule, Preset, VisionConfig, AirPlayConfig, Settings, Health, etc.) plus request/response types.

### Changed
- All 8 Svelte components updated to use `apiClient` instead of raw `fetch()`. Zero raw `fetch()` calls remain in any component. Config.svelte (27 calls), CameraCard.svelte (10), Library.svelte (5), Frigate.svelte (5), VisionTest.svelte (5), Broadcast.svelte (2), BroadcastBar.svelte (1), App.svelte (5).

---

## [v2.6.0] — 2026-07-29

### New UI components + CI lint gate

### Added
- **Toast** — store-based notification system (`toast.success/error/warning/show`) with `<Toaster />` renderer. Auto-dismiss with configurable duration.
- **Tooltip** — CSS hover/focus tooltip with 4 positions (top/bottom/left/right).
- **Dropdown** — click-to-open menu with outside-click + Escape dismissal, separator + destructive item support.
- **Accordion** — collapsible sections for organizing complex forms.
- **golangci-lint** action added to both `ci.yml` and `release.yml` GitHub Actions workflows. Lint version pinned to v2.12.2.

### Changed
- Toast notifications wired into Config.svelte and Library.svelte for save/delete/upload actions (camera saved, preset deleted, vision config saved, etc.). Inline status strings kept for contextual per-form feedback.

---

## [v2.5.0] — 2026-07-28

### Backend refactoring + CI improvements

### Changed
- Split large Go files: `internal/airplay/server.go`, `internal/api/handlers.go`, `internal/api/config_handlers.go`, `internal/config/model.go`.
- Extracted duplicated digest authentication logic to `internal/util/digest.go`.
- Extracted gain defaulting logic to `internal/util/audio.go`, applied to `internal/api/handlers.go` and `internal/api/stream.go`.
- Extracted 8kbps throttling utility `util.CopyAt8kBps` to `internal/util/stream.go`.
- `Makefile` updated with `lint`, `fmt`, and `vet` targets.
- `go build ./...` step added to both `ci.yml` and `release.yml` workflows.
- `AGENTS.md` updated to correct misleading Reolink camera configuration info and reflect the new file structure.

---

## [v2.2.0] — 2026-07-19

### AirPlay FairPlay decryption support

iOS 18+ negotiates FairPlay-encrypted audio (sending an `fpaeskey` in the ANNOUNCE SDP) instead of the legacy RSA-encrypted `rsaaeskey`. This release adds the FairPlay session-key derivation and audio-key decryption needed to handle that path, so iOS no longer drops the connection after ANNOUNCE.

### Added
- **FairPlay session key derivation** — `/fp-setup` step 1 now captures the mode byte from the iOS request, and step 2 derives the 16-byte FairPlay session key by AES-128-ECB-decrypting request bytes [12:28] with the mode-specific master key (from RPiPlay/UxPlay `fair_play.c`).
- **`fpaeskey` decryption** — ANNOUNCE now decrypts the audio AES key from the FairPlay-encrypted `fpaeskey` SDP blob (FPLY magic + AES-128-ECB) using the derived session key. Falls back to the legacy RSA `rsaaeskey` path when `fpaeskey` is absent.
- `Audio-Jack-Status: connected; type=analog` header on OPTIONS responses.
- `GET_PARAMETER` now returns `volume: -20.000000` when queried, so iOS volume UI works.

### Changed
- ANNOUNCE AES-key extraction is now a two-branch dispatch (FairPlay `fpaeskey` first, then legacy RSA `rsaaeskey`), replacing the RSA-only path.

### Notes
- The `ap-dev/` standalone AirPlay debug receiver and its `make airplay-dev` / `make airplay-dev-modern` targets are preserved on the `airplay-dev-working` tag but are not part of the released image.

---

## [v2.0.0] — 2026-07-18

### Major release — AirPlay, stop button, and library management

This release introduces three major features that significantly expand camspeak's capabilities: AirPlay receiver support, audio stop controls, and library preset renaming.

### Added
- **AirPlay v1 (RAOP) receiver** — each camera can now appear as an AirPlay target in the iOS AirPlay picker. AirPlay audio from an iPhone/iPad/Mac is decoded (ALAC → PCM → G.711ulaw) and sent to the camera speaker in real-time.
  - Pure Go implementation using the classic AirPlay v1 protocol (RAOP)
  - RSA challenge-response authentication (AirPort Express key)
  - AES-128-CBC audio decryption
  - ALAC decoding via `alicebob/alac` (pure Go, no CGO)
  - mDNS/Bonjour service advertisement via `grandcat/zeroconf`
  - Compatible with iOS 18+ and iOS 26
  - Config: `CAMSPEAK_AIRPLAY_ENABLED=true` and `CAMSPEAK_AIRPLAY_BASE_PORT=5000` env vars, or via UI Config → AirPlay tab
  - REST API: `GET/PUT /api/config/airplay`
  - Each camera gets its own RAOP listener on a sequential port starting from base_port
  - 23 tests covering RSA, SDP parsing, RTSP handshake, AES, ALAC, and full integration

- **Library preset rename** — rename presets and move them between categories from the UI.
  - Pencil button on each preset opens inline edit form with name and category fields
  - `PATCH /api/library/:category/:name` endpoint
  - File is moved on disk and metadata updated in SQLite atomically with rollback on failure
  - 5 tests covering rename, category change, conflict detection, not found, and no-op

### Tests
- 28 tests total (23 AirPlay, 5 library rename)
- AirPlay coverage: 66.4% (untested parts are UDP timing loops and ffmpeg pipeline)
- Library coverage: 50.0% (untested parts require ffmpeg)

---

## [v1.13.0] — 2026-07-17

### Added
- **STOP button** — immediately stop audio playback on all cameras or a specific camera. A red STOP button is always visible in the header bar (works from any tab). Per-camera stop buttons are on each camera card. The Broadcast tab also has a "Stop All" button.
  - `POST /api/stop` endpoint — accepts `{camera: "name"}` to stop one camera, or empty body to stop all cameras
  - Hikvision: closes the active TCP connection + ISAPI two-way audio session
  - ONVIF: closes the active RTSP client
  - go2rtc: cancels the active HTTP request + sends stop command to go2rtc
  - Reolink: no-op (audio not yet implemented)

---

## [v1.12.2] — 2026-07-17

### Added
- **Markdown rendering in vision prompt history** — vision models can return markdown (headers, lists, bold, code blocks, tables). Descriptions in the prompt history are now rendered as formatted markdown instead of plain text. Uses the `marked` library with theme-aware styling.

---

## [v1.12.1] — 2026-07-17

### Changed
- **Syntax-highlighted JSON everywhere** — Config overview, REST docs, and MCP docs now use the `JsonCode` highlighter instead of plain `<pre>` blocks. Keys, strings, numbers, booleans, and punctuation are colored (theme-aware).

---

## [v1.12.0] — 2026-07-17

### Added
- **Vision prompt presets** — save and reload named vision prompts in the Vision Test tab. Stored in SQLite (`vision_prompts` table). REST API: `GET/POST /api/config/vision-prompts`, `DELETE /api/config/vision-prompts/:name`.
- **Image upload in Vision Test** — upload any image file (not just camera snapshots) to test vision prompts against. The `POST /api/vision/test` endpoint now accepts `multipart/form-data` with an `image` file upload in addition to the existing JSON/base64 body format.

---

## [v1.11.0] — 2026-07-17

### Added
- **Vision Prompt Test playground** — new "Vision Test" tab for refining vision prompts against live camera snapshots without sending TTS to the camera. Capture a snapshot once, then iterate on prompts and see results instantly. Save the winning prompt as the global default.
  - `POST /api/vision/test` endpoint — captures snapshot (or reuses a client-provided base64 image) and runs a vision prompt, returning both description and image
  - Prompt history showing the last 10 results with timestamps
  - "Save as Global Default" button to push the refined prompt to config

---

## [v1.10.6] — 2026-07-17

### Fixed
- **Hover tooltip transparent background** — `--popover` CSS variable didn't exist in the theme; switched to `--card`
- **Hover tooltip not appearing** — `onmouseenter`/`onmouseleave` weren't firing reliably; switched to `onmouseover`/`onmouseout`

---

## [v1.10.5] — 2026-07-17

### Fixed
- **Hover preview not showing** — the CSS-only `:hover` approach wasn't working due to Svelte's CSS scoping. Switched to JS-based `onmouseenter`/`onmouseleave` events to toggle the tooltip.

---

## [v1.10.4] — 2026-07-17

### Fixed
- **Restored copy-curl buttons** on preset row and play-from-URL row (with hover preview)

### Changed
- **Vision prompt popup** — replaced the inline prompt input with a popup widget triggered by the MessageSquare button in the camera header. Popup has a textarea, Reset / Apply & Describe / Done buttons. The header button is highlighted when a custom prompt is active.

---

## [v1.10.3] — 2026-07-17

### Added
- **Hover preview on curl copy buttons** — mouse over any curl copy button (desktop only) to see a tooltip with the full curl command, syntax-highlighted and word-wrapped. On mobile/touch, no tooltip — just normal copy behavior.
- **CurlCode syntax highlighter** — colors `curl` keyword, flags (`-X`, `-H`, `-d`), HTTP methods, URLs, JSON keys/values, and line continuations. Theme-aware.

---

## [v1.10.2] — 2026-07-17

### Fixed
- **Camera name truncation** — removed `truncate` class so the full name displays
- **Copy button cleanup** — removed copy-curl from beep, preset, and play-url actions; kept only on TTS row and description area
- **Vision prompt moved inline** — the custom prompt input now appears in the snapshot/description area with a "Re-describe" button, instead of a header toggle
- **Tooltips** — all buttons now have descriptive `title` attributes

---

## [v1.10.1] — 2026-07-17

### Fixed
- **"Cannot find variable $state" runtime crash** — the `curl.ts` module used Svelte 5's `$state` rune in a plain `.ts` file, which the Svelte compiler doesn't process. Renamed to `curl.svelte.ts` and switched to an object pattern (`$state({ baseUrl: ... })`) for shared reactive state.

---

## [v1.10.0] — 2026-07-17

### Added
- **Swagger UI** at `/swagger` — interactive API explorer with "Try it out" enabled, CDN-hosted (no npm dep)
- **OpenAPI 3.0 spec** at `/api/openapi.json` — covers all REST endpoints with schemas and examples
- **"Swagger" tab** in the SPA navigation
- **Copy-curl buttons** on every camera action (speak, play, describe, beep, replay) — copies a ready-to-paste curl command with current form values
- **Base URL toggle** in the header (globe icon) — switch between local IP:port and public domain for generated curl commands; persists to localStorage
- **Copy-curl per event** in the Events tab — reconstructs the curl for any past action
- **YAML syntax highlighting** on the Home Assistant tab (new `YamlCode` component, theme-aware)
- **Reusable UI components** — `CopyButton`, `VoiceSelect`, `GainSlider` (DRY refactor)

### Changed
- **SPA routing** — each tab now has its own URL (`#/cameras`, `#/config`, etc.); reload preserves the active tab; browser back/forward navigates between tabs
- **Events tab** — long text now word-wraps instead of truncating
- **Home Assistant tab** — copy buttons replaced with reusable `CopyButton` component
- **CameraCard** — voice dropdown and gain slider replaced with reusable components

---

## [v1.9.0] — 2026-07-17

### Added
- **Home Assistant integration** — new "Home Assistant" tab with copy-paste-ready YAML snippets for `rest_command` setup, Frigate-triggered automations, webhook triggers, and dashboard buttons; corresponding README section
- **Vision config UI** — new "Vision" sub-tab in Config for endpoint URL, model, API key, and a global default prompt; persisted to SQLite with `CAMSPEAK_VISION_*` env overrides
- **`GET/PUT /api/config/vision`** — read and update vision endpoint config at runtime (PUT rebuilds the vision client)
- **`CAMSPEAK_VISION_PROMPT`** env var for the global default vision prompt
- **Syntax highlighting** on the Frigate MQTT reference payloads (dependency-free tokenizer, theme-aware)
- **Dependabot** config for `github-actions` ecosystem (weekly, grouped by org)
- **Clear button** (X) on the camera snapshot/description overlay

### Changed
- **Describe (eye) button now auto-plays TTS** — calls `/api/describe` (snapshot → vision → TTS → speak on camera) instead of `/api/vision`; no longer populates the TTS input box with the description
- **Replay button** on the vision result — play icon next to the description re-speaks it via `/api/speak` without re-running the vision model
- **Vision prompt fallback chain fixed** — request prompt → camera's `vision_prompt` → global default → hardcoded (previously the camera's prompt was ignored by the REST API)
- **CI actions bumped to Node 24** — `actions/checkout@v5`, `actions/setup-go@v6`, `actions/cache@v5`

---

## [v1.8.0] — 2025-07-16

### Added
- **MQTT topic tree browser** — live split-panel UI: topic tree on the left (collapsible, with message counts and last-value preview), message feed on the right; click any leaf topic to filter the feed
- **Auto-subscribe to `frigate/#`** on browser open for full Frigate topic discovery; all seen topics populate the datalist autocomplete
- **`GET /api/mqtt/topics`** — returns all topics seen by the broker since startup (count, last payload, timestamp)
- **`POST /api/mqtt/subscribe`** — dynamically subscribe to any topic at runtime (used internally by the browser)
- **Per-camera vision prompts** — each camera now has a saved `vision_prompt` in SQLite; Config > Cameras lets you set/edit it; CameraCard pre-fills the session prompt from the saved default (still overridable)
- **CHANGELOG.md** — this file

### Changed
- MQTT live browser now shows up to 200 messages (was 100) and includes a topic count stat
- Frigate tab reference docs updated: setup note now mentions auto-subscribe to `frigate/#`

---

## [v1.7.2] — 2025-07-16

### Fixed
- Corrected all Frigate MQTT topic suggestions and payload docs to match `docs.frigate.video/integrations/mqtt`
- Filter templates now use proper JSON objects instead of broken comma-string format
- Quick filter pills correctly serialize nested dot-notation keys

---

## [v1.7.1] — 2025-07-15

### Added
- Live MQTT message browser with SSE stream (`GET /api/mqtt/events`)
- `GET /api/mqtt/status` — broker connection status endpoint
- `SetMessageHook` on MQTT subscriber for side-channel event piping

### Fixed
- Frigate snapshot distortion — added `?h=720` to force PIL resize pipeline

---

## [v1.7.0] — 2025-07-14

### Added
- Full UI rewrite with 8 top-level tabs: Cameras, Library, Events, Broadcast, Frigate, Config, REST, MCP
- Per-camera custom vision prompts (session-scoped, via MessageSquare button on card)
- WAV / audio file drag-and-drop onto camera cards — uploads to library and plays immediately
- Full REST API documentation tab (18 endpoints with examples)
- Full MCP documentation tab with dynamic URL, Claude Desktop config snippet, and copy button
- Frigate / MQTT tab with rule management, filter templates, and payload reference
- Mobile-friendly horizontally-scrollable tab navigation

---

## [v1.6.1] — 2025-07-10

### Added
- Progressive status updates during `describe` (Capturing screenshot → Processing vision → Speaking)
- `/api/vision` endpoint — vision-only, no TTS/camera send (useful for cameras without speakers)

---

## [v1.6.0] — 2025-07-09

### Added
- UI overhaul with Lucide icons, layout polish, and visual consistency improvements

---

## [v1.5.7] — 2025-07-08

### Changed
- CI: scoped GHA cache per architecture; fixed bun cache warning

---

## [v1.5.6]

### Changed
- CI: replaced QEMU with native arm64 runners

---

## [v1.5.5]

### Fixed
- Comprehensive code review cleanup (memory leaks, resource leaks, SSRF, build consistency)

---

## [v1.5.4]

### Fixed
- Memory leaks, resource leaks, SSRF vulnerability in play-url, build consistency

---

## [v1.5.3]

### Added
- Load vision description text into TTS box for replay after describe

---

## [v1.5.2]

### Added
- Show snapshot image and description in UI after describe

---

## [v1.5.0]

### Added
- Vision describe — Frigate snapshot → vision LLM → TTS → camera

---

## [v1.4.x]

### Added
- TTS generate/preview/save flow with library management
- Play from URL (download + transcode + camera)
- Config tab: TTS presets, cameras, MQTT rules management
- Camera enable/disable with per-camera registry toggle
- go2rtc and ONVIF camera types for RTSP backchannel audio
- shadcn-svelte + Tailwind CSS v4 frontend
- Version number in UI header
- MCP endpoint (`/mcp`) for Claude Desktop integration

### Fixed
- Hikvision audio serialized with per-camera mutex
- Loudnorm removed (was undoing volume gain boost)
- Audio volume boosted 3× by default
- SPA redirect loop + SPA fallback for client-side routing
- CORS: allow all origins for LAN access

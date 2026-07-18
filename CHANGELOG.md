# Changelog

All notable changes to camspeak are documented here.
Versions follow [Semantic Versioning](https://semver.org/).

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

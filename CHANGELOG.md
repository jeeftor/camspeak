# Changelog

All notable changes to camspeak are documented here.
Versions follow [Semantic Versioning](https://semver.org/).

---

## [Unreleased] — v1.8.0

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

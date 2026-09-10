# Dashboard and reliability review implementation

Implemented September 10, 2026 for server `v4.1.0`, starting from `cdb2e6d`.
The matching Home Assistant integration has manifest version `0.15.0` and
recommends server `v4.1.0` or newer. Release publication and deployment acceptance
are separate from the implementation checks recorded below.

## Your camera screen

- Compact cards retain camera identity, readable connection status, volume,
  current playback and an optional preview. Long names wrap without colliding
  with controls. Idle cards no longer contain full forms and empty meters.
- Audio controls open one selected-camera dialog with Speak, Preset, URL and
  Describe modes. Drafts survive mode switches and closing/reopening the panel.
- Broadcast is beside the Cameras heading and uses the same composer. Partial
  failures remain visible. Arrange explicitly reveals ordering controls.
- Preparation, playing and paused states share one playback strip. Stop also
  cancels pending upload-to-play preparation; pause appears only when supported.
- One dashboard playback poll and one audio-level subscription serve all cards.
- Announce is a primary action; vision/capture/matrix experiments are grouped
  under Tools → Diagnostics. Automation/curl details are secondary controls.
- Native dialogs have readable theme colors, accessible titles and Escape/focus
  behavior. Resource failures leave navigation and settings available, with
  Retry. Stop failures produce reactive, accessible notifications. Refreshing a
  settings deep link preserves the selected page.
- TTS and Vision settings now own their drafts, loading and save/test operations
  in focused components; switching settings tabs retains drafts. Configuration
  loading no longer combines those independent resources into one failure.

## Shared implementation and correctness fixes

| Area | Delivered change |
| --- | --- |
| Audio operations | Shared preparation, cancellation and identity-aware completion across REST/MCP. Stop during download, synthesis, conversion or vision prevents delayed playback. Retired operations cannot clear or interrupt their replacements. |
| Camera edits | Serialized edits and operation publication prevent requests from reopening retired speakers. Routing is validated and persisted before active clients are replaced. Failed persistence leaves playback and routing unchanged. |
| Broadcast | Common speech is synthesized/transcoded once, then sent to targets; REST/MCP return per-camera outcomes. |
| Volume | Omitted gain inherits the camera setting; explicit zero remains mute. REST/MCP share persistence, and the UI serializes saves and rolls back failures. |
| Audio samples | Shared codec-aware silence/gain handling replaces the invalid mute byte; final partial chunks are flushed. |
| Configuration | Immutable client replacements and locked snapshots; active TTS edits update the actual client, authenticated TTS sends its key, environment overrides retain priority. |
| Credentials | Configuration reads redact secrets and report key presence. Blank edits preserve stored keys; removal is explicit. Connection tests do not forward stored keys to a different endpoint. |
| Discovery | Rediscovery preserves user overrides. Camera summaries return promptly with cached health; background probes have bounded concurrency. |
| Presets | Category/name identity is preserved through selectors, playback, broadcast and Home Assistant. Ambiguous bare-name lookup fails explicitly. |
| Uploads | One upload-and-wait helper; immutable job snapshots, a 64 MiB multipart limit, two conversion slots, timeouts and server shutdown cancellation. Playback waits for successful conversion. |
| Library safety | Central identifier validation, rooted file operations, symlink rejection, staged writes, collision-safe rename and failure preservation. |
| Preview/vision | Abortable sequential snapshots; waveform resets by source with a bounded shared cache; explicit preview-audio ownership; Describe returns its actual inference frame. |
| AirPlay | Only owned children are stopped, repeated startup failures retry with backoff, readiness reflects processes, blocked PCM can shut down, session initialization is synchronized, and camera edits/deletion rebind or retire receivers. |
| Browser security | Sanitized model Markdown, shared bounded rate limiting, same-origin browser defaults, explicit allowed origins and shell-safe copied curl arguments. |
| Release checks | Frontend checking/tests and Go race tests run in CI; multi-architecture release promotion depends on successful validation. |

## Compatibility and deliberate limits

- Continuous streams, looping and AirPlay are explicitly unavailable on camera
  backends whose implementation cannot deliver continuous audio. Finite audio
  remains supported where the backend is configured correctly. This avoids
  silently buffering an endless input; it does not add a new live transport.
- The server still has no built-in user authentication. Keep it on a trusted
  network or behind an authenticated proxy. Origin checks and redaction are not
  authentication, and the service intentionally reaches configured LAN devices.
- Deploy the matching Home Assistant integration with this server change. It
  handles preparation as buffering, capability-aware controls, omitted gain,
  explicit mute and category-qualified media identifiers.
- No framework migration, speculative generic form engine or broad package
  reorganization was introduced. Existing Svelte/Go components and protocols
  remain in use.

## Verification performed

- Full uncached Go race suite: `go test -race -count=1 -timeout=90s ./...`.
- `go vet ./...`; Go-compatible golangci-lint v2.12.2; frontend type/component
  checks with zero errors/warnings; 11 frontend regression tests; production build.
- Native executable build and `--help`; plain `make` remains help-only.
- 85 companion Home Assistant tests, plus Ruff checks, run by the integration
  reviewer against the sibling Home Assistant test environment.
- Browser checks use the production-built frontend with intercepted, synthetic
  API responses, not your real cameras. Desktop layouts were exercised with
  1, 4 and 12 cameras; narrow mobile includes long names, offline state and mute.
  Twelve cards produced one playback poll during a 3.4-second window and one
  EventSource instance. Draft retention, category-qualified requests, waveform
  source changes, dialog behavior, visible Stop errors, partial broadcast
  feedback and navigation during a voice-service failure were exercised. Settings
  checks confirm retained drafts and omitted secret fields when saving ordinary
  TTS/vision edits. Unsupported live-stream controls are disabled with an explanation.
- Regression fixtures cover traversal/symlinks/collisions, Markdown injection,
  decoded mute, trailing audio, upload races and cancellation, real TCP shutdown
  during unfinished multipart input, runtime credential/config changes, routing
  retirement, replacement ownership and AirPlay subprocess/pipe lifecycle.

## Acceptance still required on your deployment

No camera hardware, iPhone playback, production database, published container or
live Home Assistant instance was changed or exercised. Before release, verify:

1. Finite speech/presets/URLs on each camera backend you use; quiet output at
   gain zero and your saved gain after restart.
2. Hikvision live streaming and supported pause/resume; rapid replacement and
   Stop during slow preparation, followed by silence with no delayed restart.
3. Real AirPlay discovery, sustained audio, reconnect, camera add/edit/delete
   and clean shutdown on the Docker/Linux target.
4. Matched server/HACS installation, media browsing with duplicate names and
   cross-client playback/volume updates.
5. CI validation, image build and release promotion when you choose to publish.

Use `make check-frontend`, `make test-race`, `make vet` and `make lint` for your
local verification. The lint binary must be built with a Go version compatible
with the repository's Go requirement. No global tooling or TLS settings were
changed for this work.

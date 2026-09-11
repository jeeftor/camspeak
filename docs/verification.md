# Release verification

Local tests are not proof of audible camera output or a usable deployment backup.

## Automated checks

- Run `go test -race ./...` for backend, cancellation, audio conversion, and configuration regressions.
- Run `bun run check`, `bun run test`, and `bun run build` in `frontend`.
- OpenAPI tests compare registered REST methods/paths in both directions and check named and inline request fields. Some response schemas remain deliberately generic; these checks do not prove every response property's type.
- Run the companion Home Assistant client's contract tests when API behavior changes. Its administrative methods are client helpers, not newly registered Home Assistant services. Streaming methods yield raw SSE byte chunks; consumers must parse event boundaries and close the iterator when done. Playback requests must not be automatically retried after an uncertain result.

## On your camera network

1. Compare buffered and streaming speech with the same preset, text, voice, and gain. Repeat with reversed order to separate model warm-up from delivery latency. Listen for clipping, speed changes, gaps, and missing beginning/end audio.
2. Verify preset inheritance and per-camera buffered/streaming overrides after saving and restarting. Unsupported camera paths remain buffered.
3. Stop during TTS preparation, conversion, connection, and playback. Verify no later audio starts and a subsequent request works. Repeat while AirPlay is connected, checking that triggered audio has priority and AirPlay recovers afterward.
4. Test a diagnostic beep on a disabled camera; confirm ordinary speech and broadcast still exclude it.
5. Exercise Describe across a temporary browser/proxy interruption. Confirm status polling recovers without starting duplicate playback.
6. Check the timing bars with mouse, keyboard, and touch. First audio sent measures delivery to the camera, not when sound becomes audible.

## Reading recovery logs

At info level, follow `operation_id` together with `camera` and `source`. IDs are process-local and restart with the application. A normal action reserves triggered-audio priority, sends audio, finishes sending, then releases its reservation. Cancellation logs identify Stop or replacement; a prevented stale send confirms that an old request did not reopen the speaker. A release confirms cleanup, not successful or audible playback. Handler error logs still carry the request ID and failure details.

AirPlay reports a session ID (local to that audio stream), duration, and `after_trigger=true` when fresh audio reopens the camera after preemption. A receiver restart only means it is ready for a sender; it does not prove the sender reconnected or sound resumed. Camera-session failures report the retry delay. No per-chunk info logging is added.

For a failed test, retain logs from the reservation through release and the following AirPlay session, plus application version, camera name, buffered/streaming mode, and the action you took. Do not include credentials, raw audio, or private speech text. A long gap between cancellation and release points to a slow downstream cancellation/cleanup path.

## Backup and restore drill

1. Identify the actual mounted data directory (`CAMSPEAK_DATA_DIR`), any separately configured library location, and your deployment's environment/Compose settings.
2. Stop CamSpeak before copying the data directory. Copy the entire directory, including the SQLite database and library audio, plus any external library directory. Do not copy only a live `camspeak.db`: uncheckpointed data may be in its WAL. An online backup requires SQLite's backup facilities and coordinated library handling.
3. Store the backup securely: configuration and deployment files can contain credentials. Record the application version and file checksums.
4. Restore into a separate directory or isolated VM, never over your working deployment. Check database integrity and compare file checksums. Block outbound camera access and disable AirPlay before starting the restored instance.
5. Confirm cameras, presets, playback-mode overrides, and library files survived. Only then perform an explicitly authorized speaker test on your camera network.

A backup is verified only after this restore drill succeeds. Automated application tests do not establish that your VM or Docker volume has been backed up.

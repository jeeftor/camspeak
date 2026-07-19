# ap-dev: AirPlay Receiver Debug Tool

Standalone RAOP (AirPlay v1) receiver for debugging iOS connectivity.
Saves received audio to a WAV file so you can verify end-to-end receipt.
Every RTSP request/response and audio packet is logged.

## Usage

```bash
# From repo root — minimal mode (classic Airport Express style, no pk/pi)
make airplay-dev

# Modern mode (with Ed25519 pk/pi + _airplay._tcp, closer to the main app)
make airplay-dev-modern

# Or directly:
go run ./ap-dev -name "MyCamera" -port 5100 -v
go run ./ap-dev -name "MyCamera" -port 5100 -mode modern -v -play
```

## Flags

| Flag | Default | Description |
|---|---|---|
| `-name` | `AirPlay-Test` | Device name shown in iOS AirPlay picker |
| `-port` | `5100` | RTSP listener port |
| `-mode` | `minimal` | `minimal` = classic Airport Express; `modern` = with pk/pi/ft |
| `-out` | `airplay-out.wav` | Where to save received audio |
| `-play` | false | Play WAV via `afplay` after session ends (macOS) |
| `-v` | false | Verbose: dump every packet |

## Test procedure

1. Start the receiver: `make airplay-dev`
2. On your iPhone: swipe down → tap AirPlay icon → look for `AirPlay-Test`
3. Tap it — you should see the RTSP handshake logged
4. Play any audio from the iPhone
5. Stop → WAV file is saved in the current directory

## What to look for

**Device not visible in picker**: mDNS issue. Check that UDP 5353 isn't firewalled.
Try `dns-sd -B _raop._tcp local` on another Mac to see if it's being advertised.

**Device visible but tap does nothing**: iOS connected but dropped. Check the logs
for which RTSP method failed (ANNOUNCE → bad AES key, SETUP → port issue, etc.)

**Device connects but no audio**: RTP packets not arriving. Check firewall on UDP
port range. `-v` will show whether `audioLoop` receives any packets.

**Audio received but WAV sounds wrong**: ALAC decode issue. The WAV file will be
present but may be garbled — that's a decode parameter problem.

## Mode comparison

### minimal
- Only `_raop._tcp` mDNS service
- `et=0` (RSA only, no FairPlay)
- No `pk=`, `pi=`, `ft=`
- `vs=130.14` (original Airport Express)
- Best for: seeing if basic RAOP still works at all

### modern
- Both `_raop._tcp` and `_airplay._tcp`
- `ft=0x4A7FFEE6,0x0` features bitmask
- `pk=` Ed25519 public key, `pi=` pairing UUID
- `vs=366.0`
- Best for: testing the configuration closest to the main app

## Next steps

Once this receives audio successfully, the working configuration can be rolled
back into `internal/airplay/server.go`.

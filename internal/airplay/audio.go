package airplay

import (
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alicebob/alac"
	clog "github.com/charmbracelet/log"
)

// alacDecoder wraps the alicebob/alac decoder.
type alacDecoder struct {
	decoder *alac.Alac
}

// newAlacDecoder creates an ALAC decoder from the fmtp string.
// The fmtp format is: "352 0 16 40 10 14 2 255 0 0 44100"
func newAlacDecoder(fmtp string) (*alacDecoder, error) {
	// Use the alicebob/alac library which has sensible defaults for RAOP
	d, err := alac.New()
	if err != nil {
		return nil, fmt.Errorf("creating ALAC decoder: %w", err)
	}
	return &alacDecoder{decoder: d}, nil
}

// Decode decodes a single ALAC frame to 16-bit PCM.
func (d *alacDecoder) Decode(frame []byte) []byte {
	return d.decoder.Decode(frame)
}

// alacDecodeSafe calls Decode and recovers from panics in the ALAC library
// (which can occur on malformed or silence frames).
func alacDecodeSafe(d *alacDecoder, frame []byte) (pcm []byte) {
	defer func() {
		if r := recover(); r != nil {
			pcm = nil
		}
	}()
	return d.Decode(frame)
}

// audioStream manages the pipeline: PCM → ffmpeg → G.711ulaw → camera (streaming).
// ffmpeg stdout is passed directly to speaker.Stream. If the camera closes the
// connection (e.g. idle timeout), the stream goroutine reconnects automatically.
type audioStream struct {
	speaker    Speaker
	log        *clog.Logger
	ffmpegCmd  *exec.Cmd
	ffmpegIn   io.WriteCloser
	streamDone chan error
	quit       chan struct{} // closed by finish() to stop the reconnect loop
	mu         sync.Mutex

	bytesWritten int64 // bytes fed to ffmpeg stdin
	reconnects   int64 // camera speaker reconnect attempts
}

// newAudioStream starts ffmpeg and streams its output to the camera.
// PCM written via writePCM flows: ffmpeg stdin → ffmpeg stdout → speaker.Stream.
// If the camera closes the connection (e.g. idle timeout), speaker.Stream is
// called again automatically so the next audio burst works without intervention.
func newAudioStream(
	speaker Speaker,
	log *clog.Logger,
	primeMs int,
	gain float64,
) (*audioStream, error) {
	as := &audioStream{
		speaker:    speaker,
		log:        log,
		streamDone: make(chan error, 1),
		quit:       make(chan struct{}),
	}

	if gain == 0 {
		gain = 1.0
	}

	cmd := exec.Command(
		"ffmpeg",
		"-f", "s16le",
		"-ar", "44100",
		"-ac", "2",
		"-i", "pipe:0",
		"-af", fmt.Sprintf("volume=%.3f", gain),
		"-ar", "8000",
		"-ac", "1",
		"-c:a", "pcm_mulaw",
		"-f", "mulaw",
		"pipe:1",
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg stdout: %w", err)
	}
	cmd.Stderr = &lineLogger{
		log:          log,
		prefix:       "ffmpeg",
		defaultLevel: clog.DebugLevel,
		errorLevel:   clog.WarnLevel,
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting ffmpeg: %w", err)
	}

	as.ffmpegCmd = cmd
	as.ffmpegIn = stdin

	// Write prime silence — zero PCM S16LE at 44100 Hz stereo.
	// This warms the camera's audio engine so the first real audio isn't choppy.
	if primeMs > 0 {
		primeSamples := (44100 * primeMs) / 1000
		silence := make([]byte, primeSamples*4) // 4 bytes per stereo frame
		_, _ = stdin.Write(silence)
	}

	// Reconnect loop: pass ffmpeg stdout directly to speaker.Stream.
	// If the camera closes the session (idle timeout, network blip), reopen it
	// so the next audio burst reaches the camera without a manual restart.
	go func() {
		defer func() { _ = cmd.Wait() }()
		backoff := 2 * time.Second
		const maxBackoff = 30 * time.Second
		for {
			log.Info("stream: opening camera session")
			err := speaker.Stream(stdout)

			// Check whether finish() has been called before deciding to reconnect.
			select {
			case <-as.quit:
				as.streamDone <- nil
				return
			default:
			}

			if err == nil {
				// ffmpeg stdout closed cleanly — we're done.
				as.streamDone <- nil
				return
			}

			atomic.AddInt64(&as.reconnects, 1)
			log.Warn("stream: camera session lost, reconnecting", "backoff", backoff, "err", err)
			select {
			case <-time.After(backoff):
			case <-as.quit:
				as.streamDone <- nil
				return
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}()

	return as, nil
}

// writePCM feeds raw S16LE PCM into the ffmpeg transcoder.
// If ffmpeg has died (broken pipe), the pipe is closed and further writes are
// silently dropped rather than spinning on a dead process.
func (as *audioStream) writePCM(pcm []byte) {
	as.mu.Lock()
	defer as.mu.Unlock()
	if as.ffmpegIn != nil {
		if n, err := as.ffmpegIn.Write(pcm); err != nil {
			as.log.Warn("ffmpeg pipe write failed, closing pipe", "err", err)
			_ = as.ffmpegIn.Close()
			as.ffmpegIn = nil
		} else {
			atomic.AddInt64(&as.bytesWritten, int64(n))
		}
	}
}

// finish signals the reconnect loop to stop and waits for it to exit.
func (as *audioStream) finish() {
	as.mu.Lock()
	if as.ffmpegIn != nil {
		_ = as.ffmpegIn.Close()
		as.ffmpegIn = nil
	}
	as.mu.Unlock()

	// Signal reconnect loop to stop, then kill ffmpeg so stdout closes
	// and speaker.Stream returns promptly even if mid-session.
	select {
	case <-as.quit:
	default:
		close(as.quit)
	}
	if as.ffmpegCmd != nil && as.ffmpegCmd.Process != nil {
		_ = as.ffmpegCmd.Process.Kill()
	}

	select {
	case <-as.streamDone:
	case <-time.After(10 * time.Second):
		as.log.Warn("stream: timed out waiting for camera session to close")
	}
}

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
	finishOnce sync.Once
	primePCM   []byte

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
		_ = stdin.Close()
		return nil, fmt.Errorf("ffmpeg stdout: %w", err)
	}
	cmd.Stderr = &lineLogger{
		log:          log,
		prefix:       "ffmpeg",
		defaultLevel: clog.DebugLevel,
		errorLevel:   clog.WarnLevel,
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("starting ffmpeg: %w", err)
	}

	as.ffmpegCmd = cmd
	as.ffmpegIn = stdin

	// Reconnect loop: pass ffmpeg stdout directly to speaker.Stream.
	// If the camera closes the session (idle timeout, network blip), reopen it
	// so the next audio burst reaches the camera without a manual restart.
	go func() {
		defer close(as.streamDone)
		defer func() {
			_ = stdin.Close()
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}()
		backoff := 2 * time.Second
		const maxBackoff = 30 * time.Second
		const successResetThreshold = 10 * time.Second
		for {
			streamStart := time.Now()
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

			// If the session ran for a meaningful period before dropping,
			// reset the backoff — this was a healthy session, not a
			// persistent failure. Without this, repeated play/stop cycles
			// (each of which interrupts the AirPlay session) would cause
			// the backoff to grow to 30s, making the camera unresponsive.
			if elapsed := time.Since(streamStart); elapsed >= successResetThreshold {
				if backoff > 2*time.Second {
					log.Debug("stream: resetting backoff after stable session",
						"elapsed", elapsed, "prev_backoff", backoff)
				}
				backoff = 2 * time.Second
			}

			atomic.AddInt64(&as.reconnects, 1)
			log.Warn("stream: camera session lost, reconnecting",
				"backoff", backoff, "session_duration", time.Since(streamStart), "err", err)
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

	// Prime on the first write so startup returns a cancellable stream before
	// any pipe I/O can block behind an unavailable camera.
	if primeMs > 0 {
		primeSamples := (44100 * primeMs) / 1000
		as.primePCM = make([]byte, primeSamples*4)
	}

	return as, nil
}

// writePCM feeds raw S16LE PCM into the ffmpeg transcoder.
// If ffmpeg has died (broken pipe), the pipe is closed and further writes are
// silently dropped rather than spinning on a dead process.
func (as *audioStream) writePCM(pcm []byte) {
	as.mu.Lock()
	input := as.ffmpegIn
	prime := as.primePCM
	as.primePCM = nil
	as.mu.Unlock()
	if input != nil {
		if len(prime) > 0 {
			pcm = append(prime, pcm...)
		}
		if n, err := input.Write(pcm); err != nil {
			as.log.Warn("ffmpeg pipe write failed, closing pipe", "err", err)
			_ = input.Close()
			as.mu.Lock()
			if as.ffmpegIn == input {
				as.ffmpegIn = nil
			}
			as.mu.Unlock()
		} else {
			atomic.AddInt64(&as.bytesWritten, int64(n))
		}
	}
}

// finish signals the reconnect loop to stop and waits for it to exit.
func (as *audioStream) finish() {
	as.finishOnce.Do(func() {
		close(as.quit)
		as.mu.Lock()
		input := as.ffmpegIn
		as.ffmpegIn = nil
		as.mu.Unlock()
		// Cancellation must be able to break a blocked write without acquiring
		// a lock held by that writer. Only this stream's child is terminated.
		if as.ffmpegCmd != nil && as.ffmpegCmd.Process != nil {
			_ = as.ffmpegCmd.Process.Kill()
		}
		if input != nil {
			_ = input.Close()
		}
	})

	select {
	case <-as.streamDone:
	case <-time.After(10 * time.Second):
		as.log.Warn("stream: timed out waiting for camera session to close")
	}
}

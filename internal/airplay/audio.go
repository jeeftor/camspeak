package airplay

import (
	"context"
	"errors"
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
// Encoded audio is drained continuously; the camera opens only for fresh input
// and yields immediately when a triggered action reserves its speaker.
type audioStream struct {
	speaker    Speaker
	log        *clog.Logger
	ffmpegCmd  *exec.Cmd
	ffmpegIn   io.WriteCloser
	streamDone chan error
	mu         sync.Mutex
	finishOnce sync.Once
	primePCM   []byte
	cancel     context.CancelFunc

	bytesWritten int64 // bytes fed to ffmpeg stdin
	reconnects   int64 // camera speaker reconnect attempts
}

// newAudioStream starts a cancellable transcoder without occupying the camera.
// PCM written via writePCM opens a session only when triggered playback is idle.
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
	}

	cmd := exec.Command(
		"ffmpeg",
		"-probesize", "32",
		"-analyzeduration", "0",
		"-f", "s16le",
		"-ar", "44100",
		"-ac", "2",
		"-i", "pipe:0",
		"-af", fmt.Sprintf("volume=%.3f", gain),
		"-ar", "8000",
		"-ac", "1",
		"-c:a", "pcm_mulaw",
		"-f", "mulaw",
		"-flush_packets", "1",
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

	// Drain the transcoder even while a trigger owns the speaker. Never let
	// backpressure retain AirPlay audio to replay after that trigger finishes.
	ctx, cancel := context.WithCancel(context.Background())
	as.cancel = cancel
	chunks := make(chan airPlayChunk, 16) // at most 1.6 seconds of encoded audio
	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		pumpAirPlay(ctx, stdout, speaker, chunks)
	}()
	go func() {
		defer close(as.streamDone)
		defer func() {
			cancel()
			_ = stdin.Close()
			_ = cmd.Process.Kill()
			_ = stdout.Close()
			<-pumpDone
			_ = cmd.Wait()
		}()
		as.runAirPlay(ctx, chunks)
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
		if as.cancel != nil {
			as.cancel()
		}
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

// priorityAirPlaySpeaker shares the camera's reservation without changing RAOP.
type priorityAirPlaySpeaker interface {
	AirPlayState() (uint64, bool)
	BeginAirPlay(context.Context, uint64) (context.Context, func(), bool)
	StreamContext(context.Context, io.Reader) error
}

type airPlayChunk struct {
	data       []byte
	generation uint64
	at         time.Time
}

func airPlayState(speaker Speaker) (uint64, bool) {
	if prioritized, ok := speaker.(priorityAirPlaySpeaker); ok {
		return prioritized.AirPlayState()
	}
	return 0, false
}

// pumpAirPlay is the sole transcoder reader. Priority changes invalidate queued
// chunks, including audio read before a reservation but delivered after it ends.
func pumpAirPlay(ctx context.Context, source io.Reader, speaker Speaker, chunks chan airPlayChunk) {
	defer close(chunks)
	for {
		before, wasBlocked := airPlayState(speaker)
		buffer := make([]byte, 800)
		n, err := source.Read(buffer)
		if ctx.Err() != nil {
			return
		}
		generation, blocked := airPlayState(speaker)
		if n > 0 && !wasBlocked && !blocked && before == generation {
			chunk := airPlayChunk{data: buffer[:n], generation: generation, at: time.Now()}
			select {
			case chunks <- chunk:
			default:
				// Bounded latency: discard the oldest audio, never block PCM input.
				select {
				case <-chunks:
				default:
				}
				select {
				case chunks <- chunk:
				case <-ctx.Done():
					return
				}
			}
		}
		if err != nil {
			return
		}
	}
}

func (as *audioStream) runAirPlay(ctx context.Context, chunks <-chan airPlayChunk) {
	var sessionID uint64
	resumeAfterTrigger := false
	for {
		var first airPlayChunk
		select {
		case <-ctx.Done():
			return
		case chunk, ok := <-chunks:
			if !ok {
				return
			}
			first = chunk
		}
		if time.Since(first.at) > 2*time.Second {
			continue
		}
		sessionCtx, release := ctx, func() {}
		prioritized, hasPriority := as.speaker.(priorityAirPlaySpeaker)
		if hasPriority {
			var admitted bool
			sessionCtx, release, admitted = prioritized.BeginAirPlay(ctx, first.generation)
			if !admitted {
				continue
			}
		}
		reader := &airPlayReader{
			ctx: sessionCtx, chunks: chunks, pending: first.data, generation: first.generation,
		}
		sessionID++
		started := time.Now()
		as.log.Info("stream: opening camera session for incoming AirPlay audio",
			"session_id", sessionID, "generation", first.generation, "after_trigger", resumeAfterTrigger)
		resumeAfterTrigger = false
		var err error
		if hasPriority {
			err = prioritized.StreamContext(sessionCtx, reader)
		} else {
			err = as.speaker.Stream(reader)
		}
		interrupted := sessionCtx.Err() != nil
		release()
		if ctx.Err() != nil {
			as.log.Info("stream: AirPlay camera session canceled", "session_id", sessionID,
				"duration_ms", time.Since(started).Milliseconds())
			return
		}
		if interrupted {
			resumeAfterTrigger = true
			as.log.Info("stream: yielding camera speaker to triggered audio", "session_id", sessionID,
				"duration_ms", time.Since(started).Milliseconds())
			continue
		}
		if err == nil || errors.Is(err, errAirPlayIdle) {
			as.log.Info("stream: AirPlay audio idle; camera speaker released", "session_id", sessionID,
				"duration_ms", time.Since(started).Milliseconds())
			continue
		}
		atomic.AddInt64(&as.reconnects, 1)
		as.log.Warn(
			"stream: camera session lost; retrying with fresh audio",
			"err",
			err,
			"session_id",
			sessionID,
			"duration_ms",
			time.Since(started).Milliseconds(),
			"retry_delay_ms",
			2000,
		)
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

var errAirPlayIdle = errors.New("AirPlay audio idle")

// airPlayReader closes an idle session and unblocks immediately on preemption.
type airPlayReader struct {
	ctx        context.Context
	chunks     <-chan airPlayChunk
	pending    []byte
	generation uint64
}

func (r *airPlayReader) Read(p []byte) (int, error) {
	for {
		if err := r.ctx.Err(); err != nil {
			return 0, err
		}
		if len(r.pending) > 0 {
			n := copy(p, r.pending)
			r.pending = r.pending[n:]
			return n, nil
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-r.ctx.Done():
			timer.Stop()
			return 0, r.ctx.Err()
		case <-timer.C:
			return 0, errAirPlayIdle
		case chunk, ok := <-r.chunks:
			timer.Stop()
			if !ok {
				return 0, io.EOF
			}
			if chunk.generation == r.generation && time.Since(chunk.at) <= 2*time.Second {
				r.pending = chunk.data
			}
		}
	}
}

package cameras

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

// lockSpeaker waits without preempting another owner or retaining canceled work.
func lockSpeaker(ctx context.Context, mu *sync.Mutex) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if mu.TryLock() {
			if err := ctx.Err(); err != nil {
				mu.Unlock()
				return err
			}
			return nil
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// ErrLiveStreamUnsupported means this backend supports files but not live input.
var ErrLiveStreamUnsupported = errors.New(
	"your camera backend does not support live streaming or looped playback; use a finite audio file or speech",
)

// CanLiveStream reports whether a speaker consumes continuous audio immediately.
func CanLiveStream(speaker Speaker) bool {
	_, ok := speaker.(*HikvisionClient)
	return ok
}

// StreamContext cancels a live speaker session through setup and streaming.
func StreamContext(ctx context.Context, speaker Speaker, reader io.Reader) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if stream, ok := speaker.(interface {
		StreamContext(context.Context, io.Reader) error
	}); ok {
		return stream.StreamContext(ctx, reader)
	}
	return ErrLiveStreamUnsupported
}

// SendRawContext cancels preparation and playback for context-aware speakers.
func SendRawContext(
	ctx context.Context,
	speaker Speaker,
	path string,
	gain *GainController,
) (SendTiming, error) {
	if err := ctx.Err(); err != nil {
		return SendTiming{}, err
	}
	if sender, ok := speaker.(interface {
		SendRawContext(context.Context, string, *GainController) (SendTiming, error)
	}); ok {
		return sender.SendRawContext(ctx, path, gain)
	}
	return speaker.SendRaw(path, gain)
}

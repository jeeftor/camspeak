package api

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	clog "github.com/charmbracelet/log"
	"github.com/jeeftor/camspeak/internal/cameras"
)

// playbackOperation owns preparation, cancellation and completion for one camera.
type playbackOperation struct {
	id              uint64
	log             *clog.Logger
	ctx             context.Context
	cancel          context.CancelFunc
	camera          string
	cam             cameras.Speaker
	source          string
	detail          string
	onPlaying       func()
	releasePriority func()
}

var (
	operationsMu   sync.Mutex
	operations     = make(map[string]*playbackOperation)
	operationLocks sync.Map
	operationID    atomic.Uint64
)

var errCameraConfigurationChanged = errors.New("your camera configuration changed; retry playback")

// beginPlaybackOperation publishes the operation in the same critical section
// as camera edits, so a captured, retired speaker can never resume playback.
func (h *Handlers) beginPlaybackOperation(
	ctx context.Context,
	camera string,
	captured cameras.Speaker,
	source, detail string,
) (*playbackOperation, error) {
	h.configEditMu.Lock()
	defer h.configEditMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	current, err := h.reg.GetForPlayback(camera)
	if source == "beep" {
		// Resolve the current configuration under the edit lock. Disabled speakers
		// are ephemeral and are never enrolled in normal playback or broadcasts.
		current, err = h.reg.Get(camera)
	}
	if err != nil || (source != "beep" && current != captured) {
		return nil, fmt.Errorf("%w: %s", errCameraConfigurationChanged, camera)
	}
	return beginOperation(ctx, camera, current, source, detail, h.log), nil
}

func operationLock(camera string) *sync.Mutex {
	lock, _ := operationLocks.LoadOrStore(camera, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

// beginOperation replaces the previous operation before beginning preparation.
func beginOperation(
	ctx context.Context,
	camera string,
	cam cameras.Speaker,
	source, detail string,
	loggers ...*clog.Logger,
) *playbackOperation {
	lock := operationLock(camera)
	lock.Lock()
	defer lock.Unlock()
	// Reserve before cancellation or preparation: AirPlay must not reconnect
	// between stopping its session and generating the triggered audio.
	releasePriority := cameras.ReserveTriggered(cam)
	operationsMu.Lock()
	if previous := operations[camera]; previous != nil {
		previous.logLifecycle("speaker: operation superseded")
		previous.cancel()
	}
	stopStream(camera)
	ctx, cancel := context.WithCancel(ctx)
	op := &playbackOperation{
		id:              operationID.Add(1),
		ctx:             ctx,
		cancel:          cancel,
		camera:          camera,
		cam:             cam,
		source:          source,
		detail:          detail,
		releasePriority: releasePriority,
	}
	if len(loggers) > 0 {
		op.log = loggers[0]
	}
	started := time.Now()
	op.releasePriority = sync.OnceFunc(func() {
		releasePriority()
		op.logLifecycle(
			"speaker: triggered audio reservation released",
			"duration_ms",
			time.Since(started).Milliseconds(),
		)
	})
	op.logLifecycle("speaker: reserved for triggered audio")
	operations[camera] = op
	setPlayback(camera, source, detail)
	playbackStatesMu.Lock()
	playbackStates[camera].State = "preparing"
	playbackStatesMu.Unlock()
	operationsMu.Unlock()
	_ = cam.Stop()
	return op
}

// logLifecycle deliberately excludes operation detail, paths, and speech text.
func (op *playbackOperation) logLifecycle(message string, fields ...any) {
	if op.log != nil {
		op.log.Info(
			message,
			append([]any{"operation_id", op.id, "camera", op.camera, "source", op.source}, fields...)...)
	}
}

// finish only clears state owned by this operation; replaced operations are inert.
func (op *playbackOperation) finish() {
	if op.releasePriority != nil {
		defer op.releasePriority()
	}
	operationsMu.Lock()
	defer operationsMu.Unlock()
	op.cancel()
	if operations[op.camera] == op {
		delete(operations, op.camera)
		clearPlayback(op.camera)
		clearOneShotLevel(op.camera)
	}
}

// send checks ownership before opening the camera, with cancellation through setup.
func (op *playbackOperation) send(
	path string,
	gc *cameras.GainController,
) (cameras.SendTiming, error) {
	return op.sendWith(gc, func(gain *cameras.GainController) (cameras.SendTiming, error) {
		return cameras.SendRawContext(op.ctx, op.cam, path, gain)
	})
}

func (op *playbackOperation) sendWith(
	gc *cameras.GainController,
	send func(*cameras.GainController) (cameras.SendTiming, error),
) (cameras.SendTiming, error) {
	operationsMu.Lock()
	if operations[op.camera] != op || op.ctx.Err() != nil {
		operationsMu.Unlock()
		op.logLifecycle("speaker: stale or canceled send prevented")
		return cameras.SendTiming{}, context.Canceled
	}
	// Describe remains preparing until the camera accepts an audio chunk.
	if op.onPlaying == nil {
		setPlayback(op.camera, op.source, op.detail)
	}
	operationsMu.Unlock()
	var sinkMu sync.Mutex
	sinkActive := true
	if gc != nil {
		started := false
		// Use an operation-local sink so an old completion cannot detach a new one.
		gc = gc.WithLevelSink(func(level float64) {
			sinkMu.Lock()
			defer sinkMu.Unlock()
			if !sinkActive {
				return
			}
			operationsMu.Lock()
			defer operationsMu.Unlock()
			if operations[op.camera] == op {
				if !started && op.onPlaying != nil {
					started = true
					setPlayback(op.camera, op.source, op.detail)
					op.onPlaying()
				}
				setOneShotLevel(op.camera, level)
			}
		})
	}
	started := time.Now()
	op.logLifecycle("speaker: sending audio")
	timing, err := send(gc)
	// Some transports report levels on a background goroutine. Join any active
	// callback and retire the sink before the caller finalizes its result maps.
	sinkMu.Lock()
	sinkActive = false
	sinkMu.Unlock()
	if op.ctx.Err() != nil {
		op.logLifecycle("speaker: send canceled", "duration_ms", time.Since(started).Milliseconds())
		return timing, op.ctx.Err()
	}
	op.logLifecycle(
		"speaker: send finished",
		"failed",
		err != nil,
		"duration_ms",
		time.Since(started).Milliseconds(),
	)
	return timing, err
}

// stopOperation cancels preparation and prevents delayed playback after Stop.
func stopOperation(camera string) {
	operationsMu.Lock()
	defer operationsMu.Unlock()
	if op := operations[camera]; op != nil {
		op.logLifecycle("speaker: cancellation requested", "reason", "stop")
		op.cancel()
		delete(operations, camera)
		clearPlayback(camera)
	}
}

// stopAllOperations cancels every operation still preparing or playing.
func stopAllOperations() {
	operationsMu.Lock()
	defer operationsMu.Unlock()
	for camera, op := range operations {
		op.logLifecycle("speaker: cancellation requested", "reason", "stop_all")
		op.cancel()
		delete(operations, camera)
		clearPlayback(camera)
	}
}

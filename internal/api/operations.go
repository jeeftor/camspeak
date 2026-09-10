package api

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/jeeftor/camspeak/internal/cameras"
)

// playbackOperation owns preparation, cancellation and completion for one camera.
type playbackOperation struct {
	ctx    context.Context
	cancel context.CancelFunc
	camera string
	cam    cameras.Speaker
	source string
	detail string
}

var (
	operationsMu   sync.Mutex
	operations     = make(map[string]*playbackOperation)
	operationLocks sync.Map
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
	if err != nil || current != captured {
		return nil, fmt.Errorf("%w: %s", errCameraConfigurationChanged, camera)
	}
	return beginOperation(ctx, camera, current, source, detail), nil
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
) *playbackOperation {
	lock := operationLock(camera)
	lock.Lock()
	defer lock.Unlock()
	operationsMu.Lock()
	if previous := operations[camera]; previous != nil {
		previous.cancel()
	}
	stopStream(camera)
	ctx, cancel := context.WithCancel(ctx)
	op := &playbackOperation{
		ctx:    ctx,
		cancel: cancel,
		camera: camera,
		cam:    cam,
		source: source,
		detail: detail,
	}
	operations[camera] = op
	setPlayback(camera, source, detail)
	playbackStatesMu.Lock()
	playbackStates[camera].State = "preparing"
	playbackStatesMu.Unlock()
	operationsMu.Unlock()
	_ = cam.Stop()
	return op
}

// finish only clears state owned by this operation; replaced operations are inert.
func (op *playbackOperation) finish() {
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
	operationsMu.Lock()
	if operations[op.camera] != op || op.ctx.Err() != nil {
		operationsMu.Unlock()
		return cameras.SendTiming{}, context.Canceled
	}
	setPlayback(op.camera, op.source, op.detail)
	operationsMu.Unlock()
	if gc != nil {
		// Use an operation-local sink so an old completion cannot detach a new one.
		gc = gc.WithLevelSink(func(level float64) {
			operationsMu.Lock()
			defer operationsMu.Unlock()
			if operations[op.camera] == op {
				setOneShotLevel(op.camera, level)
			}
		})
	}
	timing, err := cameras.SendRawContext(op.ctx, op.cam, path, gc)
	if op.ctx.Err() != nil {
		return timing, op.ctx.Err()
	}
	return timing, err
}

// stopOperation cancels preparation and prevents delayed playback after Stop.
func stopOperation(camera string) {
	operationsMu.Lock()
	defer operationsMu.Unlock()
	if op := operations[camera]; op != nil {
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
		op.cancel()
		delete(operations, camera)
		clearPlayback(camera)
	}
}

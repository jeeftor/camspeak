package api

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/config"
)

type operationSpeaker struct{ sends int }

func (s *operationSpeaker) SendRaw(string, *cameras.GainController) (cameras.SendTiming, error) {
	s.sends++
	return cameras.SendTiming{}, nil
}

func TestRetiredSpeakerCannotPublishPlayback(t *testing.T) {
	for _, change := range []string{"routing", "edit", "disable"} {
		t.Run(change, func(t *testing.T) {
			h, _, _ := setupTestHandlers(t)
			name := "retirement-" + change
			cfg := config.CameraConfig{Type: "hikvision", IP: "127.0.0.1", Enabled: true}
			if err := h.reg.EnableCamera(name, cfg); err != nil {
				t.Fatal(err)
			}
			captured, err := h.reg.GetForPlayback(name)
			if err != nil {
				t.Fatal(err)
			}
			h.configEditMu.Lock()
			done := make(chan error, 1)
			go func() {
				op, err := h.beginPlaybackOperation(context.Background(), name, captured, "speak", "stale")
				if op != nil {
					op.finish()
				}
				done <- err
			}()
			var changeErr error
			switch change {
			case "routing":
				changeErr = h.reg.SetRouting("http://127.0.0.1:2", "")
			case "edit":
				cfg.IP = "127.0.0.2"
				changeErr = h.applyCamera(name, cfg)
			case "disable":
				cfg.Enabled = false
				changeErr = h.applyCamera(name, cfg)
			}
			h.configEditMu.Unlock()
			if changeErr != nil {
				t.Fatal(changeErr)
			}
			select {
			case err := <-done:
				if !errors.Is(err, errCameraConfigurationChanged) {
					t.Fatalf("got %v, want configuration changed", err)
				}
			case <-time.After(time.Second):
				t.Fatal("playback did not finish revalidation")
			}
			operationsMu.Lock()
			published := operations[name] != nil
			operationsMu.Unlock()
			if published {
				t.Fatal("retired speaker published playback")
			}
		})
	}
}

func TestCurrentSpeakerOperationIsRetiredByNextEdit(t *testing.T) {
	h, _, _ := setupTestHandlers(t)
	name := "current-speaker"
	cfg := config.CameraConfig{Type: "hikvision", IP: "127.0.0.1", Enabled: true}
	if err := h.reg.EnableCamera(name, cfg); err != nil {
		t.Fatal(err)
	}
	current, err := h.reg.GetForPlayback(name)
	if err != nil {
		t.Fatal(err)
	}
	op, err := h.beginPlaybackOperation(context.Background(), name, current, "speak", "preparing")
	if err != nil {
		t.Fatal(err)
	}
	defer op.finish()
	h.configEditMu.Lock()
	cfg.Enabled = false
	err = h.applyCamera(name, cfg)
	h.configEditMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if !errors.Is(op.ctx.Err(), context.Canceled) {
		t.Fatal("edit did not retire published operation")
	}
}
func (s *operationSpeaker) Stream(io.Reader) error { return nil }
func (s *operationSpeaker) Stop() error            { return nil }
func (s *operationSpeaker) Ping() bool             { return true }

func TestStopDuringPreparationPreventsPlayback(t *testing.T) {
	cam := &operationSpeaker{}
	op := beginOperation(context.Background(), "preparing-test", cam, "speak", "test")
	defer op.finish()
	stopOperation(op.camera)
	_, err := op.send("unused", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want canceled", err)
	}
	if cam.sends != 0 {
		t.Fatal("stopped preparation sent audio")
	}
}

type blockingStopSpeaker struct {
	operationSpeaker
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *blockingStopSpeaker) Stop() error {
	s.once.Do(func() { close(s.entered); <-s.release })
	return nil
}

func TestStopDuringPreviousCameraTeardownPreventsPlayback(t *testing.T) {
	cam := &blockingStopSpeaker{entered: make(chan struct{}), release: make(chan struct{})}
	ready := make(chan *playbackOperation, 1)
	go func() { ready <- beginOperation(context.Background(), "teardown-test", cam, "speak", "test") }()
	<-cam.entered
	stopOperation("teardown-test")
	close(cam.release)
	op := <-ready
	defer op.finish()
	if _, err := op.send("unused", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want canceled", err)
	}
	if cam.sends != 0 {
		t.Fatal("stopped request played after teardown finished")
	}
}

func TestOldCompletionPreservesReplacement(t *testing.T) {
	cam := &operationSpeaker{}
	old := beginOperation(context.Background(), "replacement-test", cam, "speak", "old")
	next := beginOperation(context.Background(), "replacement-test", cam, "speak", "next")
	defer next.finish()
	old.finish()
	if next.ctx.Err() != nil {
		t.Fatal("old completion canceled replacement")
	}
	if _, err := next.send("unused", nil); err != nil {
		t.Fatal(err)
	}
	state := getAllPlayback([]string{next.camera})[next.camera]
	if state.Detail != "next" || state.State != "playing" {
		t.Fatalf("replacement state lost: %+v", state)
	}
}

func TestOldStreamCleanupPreservesReplacement(t *testing.T) {
	oldCtx, oldCancel := context.WithCancel(context.Background())
	defer oldCancel()
	nextCtx, nextCancel := context.WithCancel(context.Background())
	defer nextCancel()
	old := &streamSession{cancel: oldCancel}
	next := &streamSession{cancel: nextCancel}
	activeStreamsMu.Lock()
	activeStreams["stream-replacement-test"] = next
	activeStreamsMu.Unlock()
	defer finishStream("stream-replacement-test", next)
	finishStream("stream-replacement-test", old)
	if oldCtx.Err() == nil {
		t.Fatal("old context not retired")
	}
	if nextCtx.Err() != nil {
		t.Fatal("replacement canceled by old cleanup")
	}
	activeStreamsMu.Lock()
	got := activeStreams["stream-replacement-test"]
	activeStreamsMu.Unlock()
	if got != next {
		t.Fatal("replacement removed by old cleanup")
	}
}

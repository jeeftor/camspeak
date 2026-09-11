package api

import (
	"context"
	"errors"
	"testing"
)

func TestCanceledStreamStartupReleasesPriority(t *testing.T) {
	h, _, _ := setupTestHandlers(t)
	cam := &prioritizedOperationSpeaker{}
	op := beginOperation(context.Background(), "canceled-stream-start", cam, "stream", "test")
	t.Cleanup(op.finish)
	stopOperation(op.camera)
	if err := h.startPreparedStream(h.log, op, "http://example.invalid", "http://example.invalid", 1); !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf("startup error = %v, want canceled", err)
	}
	if _, blocked := cam.priority.State(); blocked {
		t.Fatal("canceled startup retained priority without a supervisor to release it")
	}
	generation, _ := cam.priority.State()
	_, release, admitted := cam.priority.BeginAirPlay(context.Background(), generation)
	if !admitted {
		t.Fatal("AirPlay could not resume after failed startup")
	}
	release()
}

func TestReplacedStreamStartupOnlyReleasesOldPriority(t *testing.T) {
	h, _, _ := setupTestHandlers(t)
	cam := &prioritizedOperationSpeaker{}
	old := beginOperation(context.Background(), "replaced-stream-start", cam, "stream", "old")
	t.Cleanup(old.finish)
	next := beginOperation(context.Background(), old.camera, cam, "speak", "new")
	t.Cleanup(next.finish)
	if err := h.startPreparedStream(h.log, old, "http://example.invalid", "http://example.invalid", 1); !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf("startup error = %v, want canceled", err)
	}
	if _, blocked := cam.priority.State(); !blocked || next.ctx.Err() != nil {
		t.Fatal("old cleanup disturbed replacement ownership")
	}
	next.finish()
	if _, blocked := cam.priority.State(); blocked {
		t.Fatal("old startup left a reservation after replacement finished")
	}
}

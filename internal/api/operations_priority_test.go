package api

import (
	"context"
	"testing"

	"github.com/jeeftor/camspeak/internal/cameras"
)

type prioritizedOperationSpeaker struct {
	operationSpeaker
	priority cameras.AudioPriority
}

func (s *prioritizedOperationSpeaker) AudioPriority() *cameras.AudioPriority {
	return &s.priority
}

func TestPreparationReservesSpeakerBeforeAirPlayReconnect(t *testing.T) {
	cam := &prioritizedOperationSpeaker{}
	generation, _ := cam.priority.State()
	airplay, finish, ok := cam.priority.BeginAirPlay(context.Background(), generation)
	if !ok {
		t.Fatal("could not start initial AirPlay session")
	}
	op := beginOperation(context.Background(), "priority-preparation", cam, "describe", "test")
	t.Cleanup(op.finish)
	if airplay.Err() == nil {
		t.Fatal("Describe did not interrupt AirPlay before preparation")
	}
	finish()
	generation, _ = cam.priority.State()
	if _, done, admitted := cam.priority.BeginAirPlay(context.Background(), generation); admitted {
		done()
		t.Fatal("AirPlay reconnect beat unfinished Describe preparation")
	}
	if _, err := op.send("unused", nil); err != nil {
		t.Fatal(err)
	}
	op.finish()
	generation, _ = cam.priority.State()
	if _, done, admitted := cam.priority.BeginAirPlay(context.Background(), generation); admitted {
		done()
	} else {
		t.Fatal("completed Describe did not release AirPlay")
	}
}

func TestStoppedPreparationAndReplacementReleaseTheirOwnPriority(t *testing.T) {
	cam := &prioritizedOperationSpeaker{}
	old := beginOperation(context.Background(), "priority-replacement", cam, "describe", "old")
	next := beginOperation(context.Background(), "priority-replacement", cam, "beep", "new")
	t.Cleanup(next.finish)
	old.finish()
	if _, blocked := cam.priority.State(); !blocked {
		t.Fatal("canceled Describe released the replacement beep")
	}
	stopOperation(next.camera)
	next.finish()
	if _, blocked := cam.priority.State(); blocked {
		t.Fatal("Stop left a stale reservation")
	}
}

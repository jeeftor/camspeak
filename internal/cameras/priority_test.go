package cameras

import (
	"context"
	"sync"
	"testing"
)

func TestTriggeredPriorityInterruptsAndBlocksAirPlay(t *testing.T) {
	var priority AudioPriority
	generation, _ := priority.State()
	airplay, finish, ok := priority.BeginAirPlay(context.Background(), generation)
	if !ok {
		t.Fatal("idle speaker rejected AirPlay")
	}
	release := priority.Reserve()
	if airplay.Err() == nil {
		t.Fatal("trigger did not interrupt active AirPlay")
	}
	finish()
	// The old reconnect loop retried while Describe was still generating speech.
	for range 10 {
		generation, blocked := priority.State()
		if !blocked {
			t.Fatal("speaker not reserved during preparation")
		}
		if _, finish, ok := priority.BeginAirPlay(context.Background(), generation); ok {
			finish()
			t.Fatal("AirPlay stole the speaker during triggered preparation")
		}
	}
	release()
	if _, finish, ok := priority.BeginAirPlay(context.Background(), generation); ok {
		finish()
		t.Fatal("queued pre-trigger audio resumed after completion")
	}
	generation, _ = priority.State()
	_, finish, ok = priority.BeginAirPlay(context.Background(), generation)
	if !ok {
		t.Fatal("fresh AirPlay audio did not resume after completion")
	}
	finish()
}

func TestOldReservationCannotReleaseReplacement(t *testing.T) {
	var priority AudioPriority
	old := priority.Reserve()
	next := priority.Reserve()
	old()
	old()
	if _, blocked := priority.State(); !blocked {
		t.Fatal("old completion released its replacement")
	}
	next()
	next()
	if _, blocked := priority.State(); blocked {
		t.Fatal("completed replacement left a stale reservation")
	}
}

func TestConcurrentPriorityReservationAndAirPlay(t *testing.T) {
	var priority AudioPriority
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			for range 100 {
				release := priority.Reserve()
				if _, blocked := priority.State(); !blocked {
					t.Error("reservation disappeared")
				}
				release()
				generation, _ := priority.State()
				if _, finish, ok := priority.BeginAirPlay(context.Background(), generation); ok {
					finish()
				}
			}
		})
	}
	workers.Wait()
	if _, blocked := priority.State(); blocked {
		t.Fatal("concurrent completion left a stale reservation")
	}
}

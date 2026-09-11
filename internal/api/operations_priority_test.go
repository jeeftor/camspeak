package api

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	clog "github.com/charmbracelet/log"
	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/logging"
)

type prioritizedOperationSpeaker struct {
	operationSpeaker
	priority cameras.AudioPriority
}

func TestStopDuringSendAllowsReplacementAndAirPlayRecovery(t *testing.T) {
	cam := &prioritizedOperationSpeaker{}
	op := beginOperation(context.Background(), "stop-send-recovery", cam, "speak", "test")
	t.Cleanup(op.finish)
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := op.sendWith(nil, func(*cameras.GainController) (cameras.SendTiming, error) {
			close(started)
			<-op.ctx.Done()
			return cameras.SendTiming{}, op.ctx.Err()
		})
		done <- err
	}()
	<-started
	stopOperation(op.camera)
	next := beginOperation(context.Background(), op.camera, cam, "beep", "replacement")
	t.Cleanup(next.finish)
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("send did not report cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("send did not stop promptly")
	}
	op.finish()
	if _, blocked := cam.priority.State(); !blocked {
		t.Fatal("old cleanup released replacement priority")
	}
	if _, err := next.send("unused", nil); err != nil {
		t.Fatal(err)
	}
	next.finish()
	generation, _ := cam.priority.State()
	_, release, admitted := cam.priority.BeginAirPlay(context.Background(), generation)
	if !admitted {
		t.Fatal("AirPlay could not recover after replacement completed")
	}
	release()
}

func TestOperationLifecycleLogsExcludePrivateDetail(t *testing.T) {
	var output bytes.Buffer
	logger := logging.New("test", clog.InfoLevel)
	logger.SetOutput(&output)
	op := beginOperation(
		context.Background(),
		"log-test",
		&operationSpeaker{},
		"speak",
		"private speech and password",
		logger,
	)
	stopOperation(op.camera)
	_, _ = op.send("private/path", nil)
	op.finish()
	op.finish()
	logs := output.String()
	for _, expected := range []string{"operation_id", "cancellation requested", "canceled send prevented", "reservation released", "duration_ms"} {
		if !strings.Contains(logs, expected) {
			t.Errorf("missing log field or transition: %s", expected)
		}
	}
	if strings.Contains(logs, "private") || strings.Count(logs, "reservation released") != 1 {
		t.Fatal("logs disclosed private detail or duplicated release")
	}
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

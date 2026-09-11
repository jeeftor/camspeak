package airplay

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	clog "github.com/charmbracelet/log"

	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/logging"
)

type priorityTestSpeaker struct {
	priority cameras.AudioPriority
	opened   chan struct{}
	closed   chan struct{}
	played   chan []byte
}

func newPriorityTestSpeaker() *priorityTestSpeaker {
	return &priorityTestSpeaker{
		opened: make(
			chan struct{},
			8,
		), closed: make(chan struct{}, 8), played: make(chan []byte, 32),
	}
}

func (s *priorityTestSpeaker) SendRaw(string) (SendTiming, error) { return SendTiming{}, nil }
func (s *priorityTestSpeaker) Stop() error                        { return nil }
func (s *priorityTestSpeaker) Stream(r io.Reader) error {
	return s.StreamContext(context.Background(), r)
}
func (s *priorityTestSpeaker) AirPlayState() (uint64, bool) { return s.priority.State() }
func (s *priorityTestSpeaker) BeginAirPlay(
	ctx context.Context,
	generation uint64,
) (context.Context, func(), bool) {
	return s.priority.BeginAirPlay(ctx, generation)
}

func (s *priorityTestSpeaker) StreamContext(_ context.Context, r io.Reader) error {
	s.opened <- struct{}{}
	defer func() { s.closed <- struct{}{} }()
	for {
		buffer := make([]byte, 800)
		n, err := r.Read(buffer)
		if n > 0 {
			s.played <- buffer[:n]
		}
		if err != nil {
			return err
		}
	}
}

func TestAirPlayOnlyOpensForFreshAudioAndYieldsToTrigger(t *testing.T) {
	speaker := newPriorityTestSpeaker()
	as := &audioStream{speaker: speaker, log: logging.New("airplay-test", clog.ErrorLevel)}
	ctx, cancel := context.WithCancel(context.Background())
	chunks := make(chan airPlayChunk, 16)
	done := make(chan struct{})
	go func() { as.runAirPlay(ctx, chunks); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	select {
	case <-speaker.opened:
		t.Fatal("idle AirPlay occupied the camera without audio")
	case <-time.After(20 * time.Millisecond):
	}
	oldGeneration, _ := speaker.priority.State()
	chunks <- airPlayChunk{data: []byte{0x11}, generation: oldGeneration, at: time.Now()}
	select {
	case <-speaker.played:
	case <-time.After(time.Second):
		t.Fatal("incoming audio did not open the speaker")
	}
	<-speaker.opened
	release := speaker.priority.Reserve()
	select {
	case <-speaker.closed:
	case <-time.After(time.Second):
		t.Fatal("trigger could not interrupt an AirPlay reader waiting for input")
	}
	chunks <- airPlayChunk{data: []byte{0x22}, generation: oldGeneration, at: time.Now()}
	select {
	case <-speaker.opened:
		t.Fatal("AirPlay reopened while triggered audio was preparing")
	case <-time.After(20 * time.Millisecond):
	}
	release()
	// A late chunk from the old session must also be discarded after release.
	chunks <- airPlayChunk{data: []byte{0x22}, generation: oldGeneration, at: time.Now()}
	generation, _ := speaker.priority.State()
	chunks <- airPlayChunk{data: []byte{0x33}, generation: generation, at: time.Now()}
	select {
	case played := <-speaker.played:
		if !bytes.Equal(played, []byte{0x33}) {
			t.Fatalf("resumed stale pre-trigger audio: %x", played)
		}
	case <-time.After(time.Second):
		t.Fatal("fresh AirPlay audio did not resume after triggered audio finished")
	}
}

func TestAirPlayPumpDrainsAndDiscardsWhileTriggered(t *testing.T) {
	speaker := newPriorityTestSpeaker()
	release := speaker.priority.Reserve()
	chunks := make(chan airPlayChunk, 16)
	source := bytes.NewReader(bytes.Repeat([]byte{0x11}, 800*100))
	pumpAirPlay(context.Background(), source, speaker, chunks)
	if source.Len() != 0 || len(chunks) != 0 {
		t.Fatal("triggered playback retained stale AirPlay audio or blocked the transcoder")
	}
	release()

	chunks = make(chan airPlayChunk, 16)
	source = bytes.NewReader(bytes.Repeat([]byte{0x33}, 800*100))
	pumpAirPlay(context.Background(), source, speaker, chunks)
	if source.Len() != 0 || len(chunks) != cap(chunks) {
		t.Fatal("slow camera caused unbounded buffering or blocked the transcoder")
	}
}

func TestAirPlayReaderReleasesIdleCamera(t *testing.T) {
	reader := &airPlayReader{ctx: context.Background(), chunks: make(chan airPlayChunk)}
	started := time.Now()
	_, err := reader.Read(make([]byte, 800))
	if err != errAirPlayIdle || time.Since(started) > 3*time.Second {
		t.Fatalf("idle camera was not released promptly: %v", err)
	}
}

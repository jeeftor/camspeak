package cameras

import (
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeeftor/camspeak/internal/config"
)

type healthSpeaker struct {
	active, peak *atomic.Int32
	release      <-chan struct{}
}

func (s *healthSpeaker) Ping() bool {
	active := s.active.Add(1)
	for previous := s.peak.Load(); active > previous; previous = s.peak.Load() {
		if s.peak.CompareAndSwap(previous, active) {
			break
		}
	}
	<-s.release
	s.active.Add(-1)
	return true
}

func (*healthSpeaker) SendRaw(
	string,
	*GainController,
) (SendTiming, error) {
	return SendTiming{}, nil
}
func (*healthSpeaker) Stream(io.Reader) error { return nil }
func (*healthSpeaker) Stop() error            { return nil }

func TestRegistryStatusReturnsWithoutWaitingAndBoundsRefresh(t *testing.T) {
	var active, peak atomic.Int32
	release := make(chan struct{})
	r := &Registry{
		cameras: map[string]Speaker{},
		configs: map[string]config.CameraConfig{},
		health:  map[string]bool{},
	}
	for _, name := range []string{"a", "b", "c", "d", "e", "f"} {
		r.cameras[name] = &healthSpeaker{active: &active, peak: &peak, release: release}
		r.configs[name] = config.CameraConfig{Enabled: true}
	}
	done := make(chan struct{})
	go func() { r.Status(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("status waited for camera probes")
	}
	for range 20 {
		r.Status()
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if r.Status()["f"] {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if peak.Load() > 4 {
		t.Fatalf("unbounded health concurrency: %d", peak.Load())
	}
	if !r.Status()["f"] {
		t.Fatal("health refresh did not complete")
	}
}

func TestRegistryConfigIsolationAndConcurrentAccess(t *testing.T) {
	cfg := &config.Config{Go2rtcURL: "http://127.0.0.1:1", Cameras: map[string]config.CameraConfig{
		"disabled": {Type: "hikvision", IP: "127.0.0.1", Gain: 0},
	}}
	r, err := NewRegistry(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	delete(cfg.Cameras, "disabled")
	if _, err := r.Get("disabled"); err != nil {
		t.Fatal("registry config aliases caller map", err)
	}
	if len(r.Names()) != 0 {
		t.Fatal("disabled diagnostic lookup enrolled camera in broadcast")
	}
	if r.GetGain("disabled").Get() != 0 {
		t.Fatal("explicit mute changed to default gain")
	}
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			for range 50 {
				r.UpdateConfig("disabled", config.CameraConfig{Type: "hikvision"})
				r.GetGain("disabled").Set(0)
				r.Names()
				r.Status()
			}
		})
	}
	workers.Wait()
}

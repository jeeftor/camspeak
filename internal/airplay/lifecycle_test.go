package airplay

import (
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"testing"
	"time"

	clog "github.com/charmbracelet/log"

	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/logging"
)

type testReceiver struct {
	running bool
	stops   int
}

func (r *testReceiver) Start() error             { r.running = true; return nil }
func (r *testReceiver) Stop()                    { r.running = false; r.stops++ }
func (r *testReceiver) SetLogLevel(_ clog.Level) {}
func (r *testReceiver) IsRunning() bool          { return r.running }

func TestManagerOwnsConfigAndRetiresReceiver(t *testing.T) {
	cfg := &config.Config{
		AirPlay: config.AirPlayConfig{BasePort: 5100},
		Cameras: map[string]config.CameraConfig{"yard": {Gain: 0}},
	}
	m := NewManager(cfg, nil)
	cfg.Cameras["yard"] = config.CameraConfig{Gain: 9}
	if got := m.cfg.Cameras["yard"].Gain; got != 0 {
		t.Fatalf("manager shares mutable config: gain = %v", got)
	}
	r := &testReceiver{running: true}
	m.receivers["yard"] = r
	if err := m.UpdateCamera("yard", config.CameraConfig{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if r.stops != 1 || m.IsRunning("yard") {
		t.Fatal("disabling the camera did not retire its receiver")
	}
	m.RemoveCamera("yard")
	if _, ok := m.Status()["yard"]; ok {
		t.Fatal("deleted camera still has a receiver reservation")
	}
}

func TestManagerAssignsNewPortsAndReportsReadiness(t *testing.T) {
	m := NewManager(&config.Config{
		AirPlay: config.AirPlayConfig{BasePort: 65534},
		Cameras: map[string]config.CameraConfig{"existing": {}},
	}, nil)
	port, err := m.assignPort("added")
	if err != nil || port != 65535 {
		t.Fatalf("new camera port = %d, %v", port, err)
	}
	if again, err := m.assignPort("added"); err != nil || again != port {
		t.Fatalf("port changed across enable: %d, %v", again, err)
	}
	if _, err := m.assignPort("overflow"); err == nil {
		t.Fatal("allocated an invalid port above 65535")
	}
	m.receivers["added"] = &testReceiver{running: false}
	if m.IsRunning("added") || m.Status()["added"] {
		t.Fatal("retrying receiver reported ready based on map membership")
	}
}

func TestManagerSkipsUnsupportedReceiverButReportsExplicitEnable(t *testing.T) {
	cam := config.CameraConfig{Type: "go2rtc", Stream: "yard", Enabled: true, AirPlayEnabled: true}
	cfg := &config.Config{
		AirPlay:   config.AirPlayConfig{Enabled: true, BasePort: 5100},
		Cameras:   map[string]config.CameraConfig{"yard": cam},
		Go2rtcURL: "http://127.0.0.1:1984",
	}
	reg, err := cameras.NewRegistry(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(cfg, reg)
	t.Cleanup(m.Stop)
	if err := m.UpdateCamera("yard", cam); err != nil {
		t.Fatalf("ordinary camera update failed: %v", err)
	}
	if err := m.UpdateConfig(cfg.AirPlay); err != nil {
		t.Fatalf("ordinary global update failed: %v", err)
	}
	if m.IsRunning("yard") {
		t.Fatal("unsupported receiver is advertised")
	}
	if err := m.Enable("yard"); !errors.Is(err, cameras.ErrLiveStreamUnsupported) {
		t.Fatalf("explicit enable error = %v", err)
	}
}

type blockingPCMWriter struct {
	entered chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func (w *blockingPCMWriter) Write(_ []byte) (int, error) {
	close(w.entered)
	<-w.closed
	return 0, io.ErrClosedPipe
}

func (w *blockingPCMWriter) Close() error {
	w.once.Do(func() { close(w.closed) })
	return nil
}

func TestAudioFinishUnblocksPCMWriter(t *testing.T) {
	w := &blockingPCMWriter{entered: make(chan struct{}), closed: make(chan struct{})}
	as := &audioStream{
		ffmpegIn: w, quit: make(chan struct{}), streamDone: make(chan error),
		log: logging.New("airplay-test", clog.ErrorLevel),
	}
	close(as.streamDone)
	written := make(chan struct{})
	go func() { as.writePCM([]byte{1, 2}); close(written) }()
	<-w.entered
	finished := make(chan struct{})
	go func() { as.finish(); as.finish(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(time.Second):
		_ = w.Close()
		t.Fatal("finish waited behind the blocked pipe write")
	}
	select {
	case <-written:
	case <-time.After(time.Second):
		t.Fatal("PCM writer was not canceled")
	}
}

type captureSpeaker struct{ output chan []byte }

func (s *captureSpeaker) SendRaw(_ string) (SendTiming, error) { return SendTiming{}, nil }
func (s *captureSpeaker) Stop() error                          { return nil }
func (s *captureSpeaker) Stream(r io.Reader) error {
	data, err := io.ReadAll(r)
	s.output <- data
	return err
}

func TestAudioStreamPreservesExplicitMute(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	speaker := &captureSpeaker{output: make(chan []byte, 1)}
	as, err := newAudioStream(speaker, logging.New("airplay-test", clog.ErrorLevel), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(as.finish)
	pcm := make([]byte, 44100*4)
	for i := range pcm {
		pcm[i] = 0x40
	}
	as.writePCM(pcm)
	as.mu.Lock()
	input := as.ffmpegIn
	as.mu.Unlock()
	_ = input.Close()
	select {
	case output := <-speaker.output:
		if len(output) == 0 {
			t.Fatal("transcoder produced no audio")
		}
		for _, sample := range output {
			if sample != 0xff && sample != 0x7f {
				t.Fatalf("mute produced non-silent G.711 sample %#x", sample)
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("transcoder did not finish")
	}
}

// TestShairportProcess runs as a subprocess to exercise readiness and restart.
func TestShairportProcess(t *testing.T) {
	if os.Getenv("CAMSPEAK_AIRPLAY_TEST_CHILD") != "1" {
		return
	}
	dir := os.Getenv("CAMSPEAK_AIRPLAY_TEST_DIR")
	countPath := filepath.Join(dir, "attempts")
	countBytes, _ := os.ReadFile(countPath)
	count, _ := strconv.Atoi(string(countBytes))
	if err := os.WriteFile(countPath, []byte(strconv.Itoa(count+1)), 0o600); err != nil {
		os.Exit(2)
	}
	if _, err := os.Stat(filepath.Join(dir, "fail")); err == nil {
		os.Exit(1)
	}
	body, err := os.ReadFile(os.Args[len(os.Args)-1])
	if err != nil {
		os.Exit(2)
	}
	match := regexp.MustCompile(`port = (\d+);`).FindSubmatch(body)
	if len(match) != 2 {
		os.Exit(2)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:"+string(match[1]))
	if err != nil {
		os.Exit(2)
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			os.Exit(0)
		}
		_ = conn.Close()
	}
}

func TestShairportRetriesAfterFailedRelaunch(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	dir := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nexec \"$CAMSPEAK_AIRPLAY_TEST_BINARY\" -test.run=^TestShairportProcess$ -- \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "shairport-sync"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("CAMSPEAK_AIRPLAY_TEST_CHILD", "1")
	t.Setenv("CAMSPEAK_AIRPLAY_TEST_BINARY", executable)
	t.Setenv("CAMSPEAK_AIRPLAY_TEST_DIR", dir)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	s, err := NewShairportServer("test", port, "", &mockSpeaker{}, "test", 1)
	if err != nil {
		t.Fatal(err)
	}
	s.SetLogLevel(clog.ErrorLevel)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Stop)
	if !s.IsRunning() {
		t.Fatal("listener is not ready after Start")
	}
	if err := os.WriteFile(filepath.Join(dir, "fail"), []byte("fail"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	cmd := s.cmd
	s.mu.Unlock()
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 6*time.Second, func() bool {
		data, _ := os.ReadFile(filepath.Join(dir, "attempts"))
		count, _ := strconv.Atoi(string(data))
		return count >= 2
	})
	if s.IsRunning() {
		t.Fatal("failed relaunch reported running")
	}
	if err := os.Remove(filepath.Join(dir, "fail")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 12*time.Second, s.IsRunning)
	s.Stop()
	if s.IsRunning() {
		t.Fatal("stopped receiver reported running")
	}
	if _, err := os.Stat(s.workDir); !os.IsNotExist(err) {
		t.Fatalf("receiver scratch directory remains: %v", err)
	}
}

func waitFor(t *testing.T, timeout time.Duration, ready func() bool) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ready() {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("condition did not become true within %s", timeout)
		}
	}
}

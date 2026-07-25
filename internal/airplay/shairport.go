package airplay

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	clog "github.com/charmbracelet/log"
	"github.com/jeeftor/camspeak/internal/logging"
)

// ShairportServer wraps a shairport-sync subprocess and feeds its PCM output
// into the same audioStream pipeline as the pure-Go Server.
//
// shairport-sync handles all RAOP/FairPlay protocol details (including ALAC
// decryption and decoding). It outputs raw S16LE PCM at 44100 Hz stereo to
// stdout, which we read and pass to ffmpeg for transcoding to G.711 ulaw.
//
// Requires shairport-sync built with --with-stdout and --with-tinysvcmdns
// (for mDNS advertisement). In Docker, use --net=host so tinysvcmdns can
// join the LAN multicast group (224.0.0.251).
type ShairportServer struct {
	name       string
	port       int
	model      string
	gain       float64
	speaker    Speaker
	log        *clog.Logger
	pidPath    string
	configPath string

	mu     sync.Mutex
	cmd    *exec.Cmd
	stream *audioStream
	quit   chan struct{} // closed by Stop() to signal the monitor goroutine
}

// NewShairportServer creates a ShairportServer for the given camera.
// The name appears in the iOS AirPlay picker. advertiseIP is accepted for
// interface compatibility but is not used — tinysvcmdns determines the
// advertised IP.
func NewShairportServer(
	name string, port int, advertiseIP string, speaker Speaker, model string, gain float64,
) (*ShairportServer, error) {
	safeName := strings.NewReplacer(" ", "-", "/", "-", "\\", "-").Replace(
		strings.ToLower(name),
	)
	log := logging.New("shairport", clog.InfoLevel).With("camera", name)
	if advertiseIP != "" {
		log.Warn(
			"advertiseIP is ignored by shairport-sync (tinysvcmdns picks the IP)",
			"advertiseIP",
			advertiseIP,
		)
	}
	return &ShairportServer{
		name:       name,
		port:       port,
		model:      model,
		gain:       gain,
		speaker:    speaker,
		pidPath:    fmt.Sprintf("/tmp/shairport-%s-%d.pid", safeName, port),
		configPath: fmt.Sprintf("/tmp/shairport-%s-%d.conf", safeName, port),
		log:        log,
		quit:       make(chan struct{}),
	}, nil
}

// SetLogLevel changes the log level for this server.
func (s *ShairportServer) SetLogLevel(level clog.Level) {
	logging.SetLevel(s.log, level)
}

// KillAllStale kills every shairport-sync process on this host.
// Called once at manager startup to clear any leftover processes from a
// previous unclean exit, regardless of which port they were using.
func KillAllStale() {
	// Best-effort: ignore errors (process may not exist or pkill unavailable).
	if err := exec.Command("pkill", "-x", "shairport-sync").Run(); err == nil {
		// Remove any stale PID files so Start() doesn't try to re-kill them.
		_ = exec.Command("sh", "-c", "rm -f /tmp/shairport-*.pid").Run()
	}
}

// Start launches shairport-sync and starts reading PCM into the audio pipeline.
// A monitor goroutine reaps the subprocess on exit and auto-restarts on crash.
func (s *ShairportServer) Start() error {
	// Kill any stale instance left over from a previous unclean exit.
	s.killStalePID()

	if err := s.launchProcess(); err != nil {
		return err
	}

	// Monitor goroutine: waits for subprocess exit, reaps zombie, restarts on crash.
	go s.monitor()

	return nil
}

// launchProcess starts shairport-sync and the PCM reader goroutine.
// Called both from Start() and from the monitor loop on restart.
func (s *ShairportServer) launchProcess() error {
	stream, err := newAudioStream(s.speaker, s.log, 0, s.gain)
	if err != nil {
		return fmt.Errorf("audio stream: %w", err)
	}

	// Write a minimal shairport-sync config so we can control the advertised
	// model string (am= / model=), which determines the icon in the iOS picker.
	configBody := fmt.Sprintf(`general = {
  name = "%s";
  port = %d;
  model = "%s";
  output_backend = "stdout";
};
`, s.name, s.port, s.model)
	if err := os.WriteFile(s.configPath, []byte(configBody), 0o644); err != nil {
		stream.finish()
		return fmt.Errorf("writing shairport-sync config: %w", err)
	}

	cmd := exec.Command(
		"shairport-sync",
		"-c", s.configPath,
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stream.finish()
		return fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = &lineLogger{log: s.log, prefix: "shairport-sync", defaultLevel: clog.InfoLevel}

	if err := cmd.Start(); err != nil {
		stream.finish()
		return fmt.Errorf("starting shairport-sync: %w", err)
	}

	_ = os.WriteFile(s.pidPath, fmt.Appendf(nil, "%d\n", cmd.Process.Pid), 0o644)

	s.mu.Lock()
	s.cmd = cmd
	s.stream = stream
	s.mu.Unlock()

	s.log.Info("shairport-sync started", "port", s.port, "pid", cmd.Process.Pid)

	// Read PCM from shairport-sync stdout -> audio pipeline.
	go func() {
		buf := make([]byte, 8192)
		totalBytes := 0
		for {
			n, readErr := stdout.Read(buf)
			if n > 0 {
				if totalBytes == 0 {
					s.log.Info("shairport-sync: first PCM data received — audio is flowing")
				}
				totalBytes += n
				stream.writePCM(buf[:n])
			}
			if readErr != nil {
				if totalBytes > 0 {
					s.log.Info("shairport-sync stdout closed", "total_pcm_bytes", totalBytes)
				} else {
					s.log.Info("shairport-sync stdout closed (no audio received)")
				}
				return
			}
		}
	}()

	return nil
}

// monitor waits for the shairport-sync subprocess to exit. If Stop() was not
// called (i.e. the process crashed), it cleans up and restarts after a delay.
func (s *ShairportServer) monitor() {
	for {
		s.mu.Lock()
		cmd := s.cmd
		stream := s.stream
		s.mu.Unlock()

		if cmd == nil {
			return // Stop() already cleaned up
		}

		// Reap the subprocess — blocks until it exits.
		waitErr := cmd.Wait()

		// Check if Stop() was called.
		select {
		case <-s.quit:
			return
		default:
		}

		// Subprocess exited unexpectedly — log, clean up, and restart.
		s.log.Warn("shairport-sync crashed, will restart in 3s", "exit", waitErr)

		// Clean up the old audio stream.
		if stream != nil {
			stream.finish()
		}
		s.mu.Lock()
		s.cmd = nil
		s.stream = nil
		s.mu.Unlock()
		_ = os.Remove(s.pidPath)

		// Wait before restart, but bail if Stop() is called.
		select {
		case <-time.After(3 * time.Second):
		case <-s.quit:
			return
		}

		s.killStalePID()
		if err := s.launchProcess(); err != nil {
			s.log.Error("shairport-sync restart failed", "err", err)
			// Back off longer on repeated failures.
			select {
			case <-time.After(10 * time.Second):
			case <-s.quit:
				return
			}
		}
	}
}

// killStalePID reads the PID file and kills the process if it's still running.
// Called at Start() to clean up processes left over from unclean exits.
func (s *ShairportServer) killStalePID() {
	data, err := os.ReadFile(s.pidPath)
	if err != nil {
		return // no PID file, nothing to do
	}
	pidStr := strings.TrimSpace(string(data))
	pid := 0
	if _, err := fmt.Sscan(pidStr, &pid); err != nil || pid <= 0 {
		_ = os.Remove(s.pidPath)
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		_ = os.Remove(s.pidPath)
		return
	}
	if err := proc.Kill(); err == nil {
		s.log.Info("killed stale shairport-sync", "pid", pid)
		_, _ = proc.Wait()
	}
	_ = os.Remove(s.pidPath)
}

// Stop kills the shairport-sync subprocess and cleans up.
func (s *ShairportServer) Stop() {
	// Signal the monitor goroutine to stop before taking the lock,
	// so it won't try to restart after we kill the process.
	select {
	case <-s.quit:
	default:
		close(s.quit)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		_ = s.cmd.Wait()
		s.cmd = nil
	}
	if s.stream != nil {
		s.stream.finish()
		s.stream = nil
	}
	_ = os.Remove(s.pidPath)
	_ = os.Remove(s.configPath)
	s.log.Info("shairport-sync stopped")
}

// lineLogger forwards subprocess stderr to our structured logger line by line.
// Normal lines are emitted at defaultLevel; lines that look like errors are
// promoted to errorLevel so problems are always visible without startup spam.
type lineLogger struct {
	log          *clog.Logger
	buf          []byte
	prefix       string
	defaultLevel clog.Level
	errorLevel   clog.Level
}

func (l *lineLogger) Write(p []byte) (int, error) {
	l.buf = append(l.buf, p...)
	for {
		idx := bytes.IndexByte(l.buf, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimSpace(string(l.buf[:idx]))
		if line != "" {
			level := l.defaultLevel
			if l.errorLevel != 0 && looksLikeError(line) {
				level = l.errorLevel
			}
			switch level {
			case clog.DebugLevel:
				l.log.Debug(l.prefix, "msg", line)
			case clog.InfoLevel:
				l.log.Info(l.prefix, "msg", line)
			case clog.WarnLevel:
				l.log.Warn(l.prefix, "msg", line)
			case clog.ErrorLevel:
				l.log.Error(l.prefix, "msg", line)
			default:
				l.log.Info(l.prefix, "msg", line)
			}
		}
		l.buf = l.buf[idx+1:]
	}
	return len(p), nil
}

// looksLikeError returns true for stderr lines that indicate a real problem
// rather than normal startup/progress output.
func looksLikeError(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "error") ||
		strings.Contains(lower, "failed") ||
		strings.Contains(lower, "fatal") ||
		strings.Contains(lower, "invalid") ||
		strings.Contains(lower, "cannot") ||
		strings.Contains(lower, "unable") ||
		strings.Contains(lower, "not found")
}

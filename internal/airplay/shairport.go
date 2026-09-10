package airplay

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	name           string
	port           int
	model          string
	gain           float64
	primeSilenceMs int
	speaker        Speaker
	log            *clog.Logger
	workDir        string
	configPath     string

	mu          sync.Mutex
	cmd         *exec.Cmd
	stream      *audioStream
	quit        chan struct{} // closed by Stop() to signal the monitor goroutine
	done        chan struct{} // closed when the monitor has stopped and reaped its child
	processDone chan struct{}
	stopOnce    sync.Once
	running     atomic.Bool
}

// NewShairportServer creates a ShairportServer for the given camera.
// The name appears in the iOS AirPlay picker. advertiseIP is accepted for
// interface compatibility but is not used — tinysvcmdns determines the
// advertised IP.
func NewShairportServer(
	name string, port int, advertiseIP string, speaker Speaker, model string, gain float64,
) (*ShairportServer, error) {
	log := logging.New("shairport", clog.InfoLevel).With("camera", name)
	if advertiseIP != "" {
		log.Warn(
			"advertiseIP is ignored by shairport-sync (tinysvcmdns picks the IP)",
			"advertiseIP",
			advertiseIP,
		)
	}
	return &ShairportServer{
		name:    name,
		port:    port,
		model:   model,
		gain:    gain,
		speaker: speaker,
		log:     log,
		quit:    make(chan struct{}),
		done:    make(chan struct{}),
	}, nil
}

// SetLogLevel changes the log level for this server.
func (s *ShairportServer) SetLogLevel(level clog.Level) {
	logging.SetLevel(s.log, level)
}

// Start launches shairport-sync and starts reading PCM into the audio pipeline.
// A monitor goroutine reaps the subprocess on exit and auto-restarts on crash.
func (s *ShairportServer) Start() error {
	// Private scratch paths avoid following stale PID/config files or taking
	// ownership of another service's processes after a restart.
	workDir, err := os.MkdirTemp("", "camspeak-shairport-")
	if err != nil {
		return fmt.Errorf("creating shairport-sync directory: %w", err)
	}
	s.workDir = workDir
	s.configPath = filepath.Join(workDir, "shairport.conf")
	if err := s.launchProcess(); err != nil {
		s.cleanupFiles()
		close(s.done)
		return err
	}

	// Monitor goroutine: waits for subprocess exit, reaps zombie, restarts on crash.
	go s.monitor()

	return nil
}

// launchProcess starts shairport-sync and the PCM reader goroutine.
// Called both from Start() and from the monitor loop on restart.
func (s *ShairportServer) launchProcess() error {
	select {
	case <-s.quit:
		return fmt.Errorf("receiver is stopped")
	default:
	}
	// Refuse occupied ports before starting audio or inspecting readiness, so
	// another service's listener can never make this receiver appear healthy.
	probe, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("reserving shairport-sync port %d: %w", s.port, err)
	}
	_ = probe.Close()
	stream, err := newAudioStream(s.speaker, s.log, s.primeSilenceMs, s.gain)
	if err != nil {
		return fmt.Errorf("audio stream: %w", err)
	}

	// Write a minimal shairport-sync config so we can control the advertised
	// model string (am= / model=), which determines the icon in the iOS picker.
	configBody := fmt.Sprintf(`general = {
  name = %s;
  port = %d;
  model = %s;
  output_backend = "stdout";
};
`, strconv.Quote(s.name), s.port, strconv.Quote(s.model))
	if err := os.WriteFile(s.configPath, []byte(configBody), 0o600); err != nil {
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

	processDone := make(chan struct{})
	s.mu.Lock()
	s.cmd = cmd
	s.stream = stream
	s.processDone = processDone
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
	// One goroutine owns Wait. Stop and the monitor only signal this child;
	// they never compete to reap it.
	go func() {
		waitErr := cmd.Wait()
		s.running.Store(false)
		if waitErr != nil {
			s.log.Debug("shairport-sync exited", "err", waitErr)
		}
		close(processDone)
	}()

	if err := s.waitReady(processDone); err != nil {
		_ = cmd.Process.Kill()
		stream.finish()
		<-processDone
		s.mu.Lock()
		s.cmd = nil
		s.stream = nil
		s.mu.Unlock()
		return err
	}

	return nil
}

// waitReady waits for the owned child to accept RTSP connections, or fail.
func (s *ShairportServer) waitReady(processDone <-chan struct{}) error {
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-s.quit:
			return fmt.Errorf("receiver stopped during startup")
		case <-processDone:
			return fmt.Errorf("shairport-sync exited before opening port %d", s.port)
		case <-deadline.C:
			return fmt.Errorf("shairport-sync did not open port %d within 5 seconds", s.port)
		case <-ticker.C:
			conn, err := net.DialTimeout(
				"tcp",
				net.JoinHostPort("127.0.0.1", strconv.Itoa(s.port)),
				100*time.Millisecond,
			)
			if err == nil {
				_ = conn.Close()
				s.running.Store(true)
				// Do not report a dead child as ready if exit raced the probe.
				select {
				case <-s.quit:
					s.running.Store(false)
					return fmt.Errorf("receiver stopped during startup")
				case <-processDone:
					s.running.Store(false)
					return fmt.Errorf("shairport-sync exited during startup")
				default:
					return nil
				}
			}
		}
	}
}

// monitor waits for the shairport-sync subprocess to exit. If Stop() was not
// called (i.e. the process crashed), it cleans up and restarts after a delay.
func (s *ShairportServer) monitor() {
	defer close(s.done)
	backoff := 3 * time.Second
	for {
		s.mu.Lock()
		processDone := s.processDone
		stream := s.stream
		s.mu.Unlock()
		select {
		case <-s.quit:
			return
		case <-processDone:
		}
		s.running.Store(false)
		if stream != nil {
			stream.finish()
		}
		s.mu.Lock()
		s.cmd = nil
		s.stream = nil
		s.mu.Unlock()
		// Keep retrying launch failures. A nil process means retrying, not that
		// the receiver has been intentionally removed from the manager.
		for {
			s.log.Warn("shairport-sync stopped, retrying", "backoff", backoff)
			select {
			case <-time.After(backoff):
			case <-s.quit:
				return
			}
			if err := s.launchProcess(); err == nil {
				backoff = 3 * time.Second
				break
			} else {
				s.log.Warn("shairport-sync restart failed", "err", err)
			}
			backoff = min(backoff*2, 30*time.Second)
		}
	}
}

// IsRunning reports actual listener readiness, including false during retries.
func (s *ShairportServer) IsRunning() bool {
	if !s.running.Load() {
		return false
	}
	s.mu.Lock()
	processDone := s.processDone
	s.mu.Unlock()
	select {
	case <-s.quit:
		return false
	case <-processDone:
		return false
	default:
		return true
	}
}

// cleanupFiles removes only the files created in this receiver's private directory.
func (s *ShairportServer) cleanupFiles() {
	if s.configPath != "" {
		_ = os.Remove(s.configPath)
	}
	if s.workDir != "" {
		_ = os.Remove(s.workDir)
	}
}

// Stop kills the shairport-sync subprocess and cleans up.
func (s *ShairportServer) Stop() {
	s.stopOnce.Do(func() {
		close(s.quit)
		s.running.Store(false)
		s.mu.Lock()
		cmd, stream, processDone := s.cmd, s.stream, s.processDone
		s.mu.Unlock()
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		if stream != nil {
			stream.finish()
		}
		if processDone != nil {
			<-processDone
		}
		// A pending launch observes quit and disposes of any child it created.
		if s.workDir != "" {
			<-s.done
		}
		s.cleanupFiles()
		s.log.Info("shairport-sync stopped")
	})
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

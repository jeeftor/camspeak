package airplay

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	clog "github.com/charmbracelet/log"
)

// ShairportServer wraps a shairport-sync subprocess and feeds its PCM output
// into the same audioStream pipeline as the pure-Go Server.
//
// shairport-sync handles all RAOP/FairPlay protocol details (including ALAC
// decryption and decoding). It outputs raw S16LE PCM at 44100 Hz stereo to
// stdout, which we read and pass to ffmpeg for transcoding to G.711 ulaw.
//
// Requires shairport-sync built with --with-stdout and avahi-daemon running
// (for mDNS advertisement). In Docker, use --net=host so avahi can reach
// the LAN multicast group.
type ShairportServer struct {
	name    string
	port    int
	speaker Speaker
	log     *clog.Logger
	pidPath string

	mu     sync.Mutex
	cmd    *exec.Cmd
	stream *audioStream
}

// NewShairportServer creates a ShairportServer for the given camera.
// The name appears in the iOS AirPlay picker. advertiseIP is accepted for
// interface compatibility but is not used — avahi determines the advertised IP.
func NewShairportServer(
	name string, port int, _ string, speaker Speaker,
) (*ShairportServer, error) {
	safeName := strings.NewReplacer(" ", "-", "/", "-", "\\", "-").Replace(
		strings.ToLower(name),
	)
	return &ShairportServer{
		name:    name,
		port:    port,
		speaker: speaker,
		pidPath: fmt.Sprintf("/tmp/shairport-%s-%d.pid", safeName, port),
		log: clog.NewWithOptions(os.Stderr, clog.Options{
			Prefix:          fmt.Sprintf("shairport[%s]", name),
			ReportTimestamp: true,
			Level:           clog.InfoLevel,
		}),
	}, nil
}

// SetLogLevel changes the log level for this server.
func (s *ShairportServer) SetLogLevel(level clog.Level) {
	s.log.SetLevel(level)
}

// Start launches shairport-sync and starts reading PCM into the audio pipeline.
func (s *ShairportServer) Start() error {
	// Kill any stale instance left over from a previous unclean exit.
	s.killStalePID()

	stream, err := newAudioStream(s.speaker, s.log)
	if err != nil {
		return fmt.Errorf("audio stream: %w", err)
	}

	// Pass name and port directly as CLI flags — simpler and more reliable
	// than a config file. Use classic AirPlay 1 (RAOP) mode.
	cmd := exec.Command(
		"shairport-sync",
		"--service-type", "classic",
		"-a", s.name,
		"-p", fmt.Sprintf("%d", s.port),
		"-o", "stdout",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stream.finish()
		return fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = &lineLogger{log: s.log}

	if err := cmd.Start(); err != nil {
		stream.finish()
		return fmt.Errorf("starting shairport-sync: %w", err)
	}

	// Write PID file so we can clean up stale instances on next start.
	_ = os.WriteFile(s.pidPath, []byte(fmt.Sprintf("%d\n", cmd.Process.Pid)), 0o644)

	s.mu.Lock()
	s.cmd = cmd
	s.stream = stream
	s.mu.Unlock()

	s.log.Info("shairport-sync started", "port", s.port, "pid", cmd.Process.Pid)

	// Read PCM from shairport-sync stdout → audio pipeline.
	// When the process is killed, stdout closes and this goroutine exits.
	go func() {
		buf := make([]byte, 8192)
		totalBytes := 0
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				if totalBytes == 0 {
					s.log.Info("shairport-sync: first PCM data received — audio is flowing")
				}
				totalBytes += n
				stream.writePCM(buf[:n])
			}
			if err != nil {
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
	s.log.Info("shairport-sync stopped")
}

// lineLogger forwards subprocess stderr to our structured logger line by line.
// Connection/session events are promoted to Info; everything else is Debug.
type lineLogger struct {
	log *clog.Logger
	buf []byte
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
			// Log all stderr at Info so we can see what shairport-sync is doing.
			l.log.Info("shairport-sync", "msg", line)
		}
		l.buf = l.buf[idx+1:]
	}
	return len(p), nil
}

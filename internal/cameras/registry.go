package cameras

import (
	"fmt"
	"io"
	"net"
	"os/exec"
	"time"

	clog "github.com/charmbracelet/log"
	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/logging"
	"github.com/jeeftor/camspeak/internal/tts"
)

// LogLevel holds the global log level for camera clients. Set by cmd package
// at startup from CAMSPEAK_LOG_LEVEL env var.
var LogLevel = clog.InfoLevel

// SetLogLevel sets the log level for all camera clients (called from cmd at startup).
func SetLogLevel(level clog.Level) {
	LogLevel = level
}

// newLogger creates a charmbracelet logger with the given prefix and global LogLevel.
func newLogger(prefix string) *clog.Logger {
	return logging.New(prefix, LogLevel)
}

// Speaker is the interface all camera types implement.
type Speaker interface {
	SendRaw(rawFile string) error
	Stream(r io.Reader) error
	Ping() bool
	Stop() error
}

// Registry holds all configured cameras.
type Registry struct {
	cameras     map[string]Speaker
	configs     map[string]config.CameraConfig
	tts         *tts.Client
	go2rtcURL   string
	advertiseIP string
}

// NewRegistry builds a Registry from config.
// Only cameras with Enabled=true are registered; disabled cameras are
// loaded into configs but skipped (they won't appear in Names() or receive
// speak/broadcast).
func NewRegistry(cfg *config.Config, ttsClient *tts.Client) (*Registry, error) {
	r := &Registry{
		cameras:     make(map[string]Speaker),
		configs:     cfg.Cameras,
		tts:         ttsClient,
		go2rtcURL:   cfg.Go2rtcURL,
		advertiseIP: cfg.AdvertiseIP,
	}

	for name, cam := range cfg.Cameras {
		if !cam.Enabled {
			continue
		}
		if err := r.register(name, cam); err != nil {
			return nil, err
		}
	}

	return r, nil
}

// register creates and registers a Speaker for the given camera config.
func (r *Registry) register(name string, cam config.CameraConfig) error {
	switch cam.Type {
	case "hikvision":
		r.cameras[name] = NewHikvisionClient(cam.IP, cam.User, cam.Pass, cam.Channel)
	case "reolink":
		r.cameras[name] = NewReolinkClient(cam.IP, cam.User, cam.Pass)
	case "go2rtc":
		if r.go2rtcURL == "" {
			return fmt.Errorf("camera %q uses go2rtc type but CAMSPEAK_GO2RTC_URL is not set", name)
		}
		if cam.Stream == "" {
			return fmt.Errorf("camera %q uses go2rtc type but no stream name configured", name)
		}
		r.cameras[name] = NewGo2rtcClient(r.go2rtcURL, cam.Stream, cam.IP, r.advertiseIP)
	case "onvif":
		rtspURL := cam.Stream
		if rtspURL == "" {
			// Build RTSP URL from IP/credentials if stream not set
			if cam.User != "" && cam.Pass != "" {
				rtspURL = fmt.Sprintf("rtsp://%s:%s@%s:554/stream0", cam.User, cam.Pass, cam.IP)
			} else {
				rtspURL = fmt.Sprintf("rtsp://%s:554/stream0", cam.IP)
			}
		}
		r.cameras[name] = NewOnvifClient(rtspURL, cam.IP)
	default:
		return fmt.Errorf("unknown camera type %q for camera %q", cam.Type, name)
	}
	return nil
}

// EnableCamera registers a camera at runtime (after toggle on).
func (r *Registry) EnableCamera(name string, cam config.CameraConfig) error {
	return r.register(name, cam)
}

// DisableCamera unregisters a camera at runtime (after toggle off).
func (r *Registry) DisableCamera(name string) {
	delete(r.cameras, name)
}

// UpdateConfig updates the stored config for a camera (used after save/toggle).
func (r *Registry) UpdateConfig(name string, cam config.CameraConfig) {
	r.configs[name] = cam
}

// Get returns the Speaker for a camera name.
// If the camera is not registered (e.g. disabled for speak/broadcast) but has a
// known config, it is registered on-demand so AirPlay can reach it.
func (r *Registry) Get(name string) (Speaker, error) {
	if s, ok := r.cameras[name]; ok {
		return s, nil
	}
	// Camera may be disabled (not in r.cameras) but config is known — register on-demand.
	if cam, ok := r.configs[name]; ok {
		if err := r.register(name, cam); err != nil {
			return nil, fmt.Errorf("camera %q not registered and on-demand init failed: %w", name, err)
		}
		return r.cameras[name], nil
	}
	return nil, fmt.Errorf("camera %q not found (available: %v)", name, r.Names())
}

// Names returns all configured camera names.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.cameras))
	for name := range r.cameras {
		names = append(names, name)
	}

	return names
}

// Status returns online status for all cameras.
func (r *Registry) Status() map[string]bool {
	out := make(map[string]bool)
	for name, cam := range r.cameras {
		out[name] = cam.Ping()
	}

	return out
}

// Stop stops audio playback on a specific camera.
func (r *Registry) Stop(name string) error {
	cam, ok := r.cameras[name]
	if !ok {
		return fmt.Errorf("camera %q not found", name)
	}
	return cam.Stop()
}

// StopAll stops audio playback on all cameras.
func (r *Registry) StopAll() {
	for _, cam := range r.cameras {
		_ = cam.Stop()
	}
}

// FFmpegAvailable checks that ffmpeg is on PATH (required for transcoding).
func FFmpegAvailable() bool {
	_, err := exec.LookPath("ffmpeg")

	return err == nil
}

// tcpPing checks if a TCP port is reachable within the given timeout.
// Used as a fallback when HTTP pings fail (e.g. wrong credentials but
// camera is still on the network).
func tcpPing(ip string, port int, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, port), timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

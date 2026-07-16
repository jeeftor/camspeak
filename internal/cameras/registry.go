package cameras

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"

	clog "github.com/charmbracelet/log"
	"github.com/jeeftor/camspeak/internal/config"
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
	return clog.NewWithOptions(os.Stderr, clog.Options{
		Prefix:           prefix,
		ReportTimestamp:  true,
		Level:            LogLevel,
	})
}

// Speaker is the interface all camera types implement.
type Speaker interface {
	SendRaw(rawFile string) error
	Ping() bool
}

// Registry holds all configured cameras.
type Registry struct {
	cameras map[string]Speaker
	configs map[string]config.CameraConfig
	tts     *tts.Client
}

// NewRegistry builds a Registry from config.
func NewRegistry(cfg *config.Config, ttsClient *tts.Client) (*Registry, error) {
	r := &Registry{
		cameras: make(map[string]Speaker),
		configs: cfg.Cameras,
		tts:     ttsClient,
	}

	for name, cam := range cfg.Cameras {
		switch cam.Type {
		case "hikvision":
			r.cameras[name] = NewHikvisionClient(cam.IP, cam.User, cam.Pass, cam.Channel)
		case "reolink":
			r.cameras[name] = NewReolinkClient(cam.IP, cam.User, cam.Pass)
		case "go2rtc":
			if cfg.Go2rtcURL == "" {
				return nil, fmt.Errorf("camera %q uses go2rtc type but CAMSPEAK_GO2RTC_URL is not set", name)
			}
			if cam.Stream == "" {
				return nil, fmt.Errorf("camera %q uses go2rtc type but no stream name configured", name)
			}
			r.cameras[name] = NewGo2rtcClient(cfg.Go2rtcURL, cam.Stream, cam.IP, cfg.AdvertiseIP)
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
			return nil, fmt.Errorf("unknown camera type %q for camera %q", cam.Type, name)
		}
	}

	return r, nil
}

// Get returns the Speaker for a camera name.
func (r *Registry) Get(name string) (Speaker, error) {
	s, ok := r.cameras[name]
	if !ok {
		return nil, fmt.Errorf("camera %q not found (available: %v)", name, r.Names())
	}

	return s, nil
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

package cameras

import (
	"fmt"
	"io"
	"net"
	"os/exec"
	"sync"
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

// SendTiming breaks down the time spent inside SendRaw into latency
// (everything before the first audio byte reaches the camera) and
// playback (the throttled streaming of the rest of the audio).
type SendTiming struct {
	OpenMs     int64 // latency: channel open + auth + first audio byte
	PlaybackMs int64 // streaming duration: rest of the audio at real-time speed
}

// GainController holds a per-camera volume gain that can be updated at
// runtime (via the volume API) and read by the audio send loop per-chunk.
// gain=1.0 is unity (no change). The stored raw files are pre-boosted at
// gain=3.0 during transcoding, so the runtime gain is relative to that.
type GainController struct {
	mu   sync.RWMutex
	gain float64
}

// NewGainController creates a GainController with the given initial gain.
func NewGainController(gain float64) *GainController {
	return &GainController{gain: gain}
}

// Get returns the current gain value.
func (g *GainController) Get() float64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.gain
}

// Set updates the gain value. Safe to call while audio is playing —
// the next chunk will pick up the new value.
func (g *GainController) Set(gain float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.gain = gain
}

// Speaker is the interface all camera types implement.
type Speaker interface {
	SendRaw(rawFile string, gc *GainController) (SendTiming, error)
	Stream(r io.Reader) error
	Ping() bool
	Stop() error
}

// Registry holds all configured cameras.
type Registry struct {
	cameras     map[string]Speaker
	configs     map[string]config.CameraConfig
	gains       map[string]*GainController
	tts         *tts.Client
	go2rtcURL   string
	advertiseIP string
}

// NewRegistry builds a Registry from config.
// Only cameras with Enabled=true are registered; disabled cameras are
// loaded into configs but skipped (they won't appear in Names() or receive
// speak/broadcast).
func NewRegistry(cfg *config.Config, ttsClient *tts.Client) (*Registry, error) {
	go2rtcURL := cfg.Go2rtcURL
	if go2rtcURL == "" {
		go2rtcURL = FindGo2rtcURL(cfg.FrigateURL)
		if go2rtcURL != "" {
			newLogger("registry").Info("auto-detected go2rtc", "url", go2rtcURL)
		}
	}
	r := &Registry{
		cameras:     make(map[string]Speaker),
		configs:     cfg.Cameras,
		gains:       make(map[string]*GainController),
		tts:         ttsClient,
		go2rtcURL:   go2rtcURL,
		advertiseIP: cfg.AdvertiseIP,
	}

	for name, cam := range cfg.Cameras {
		gain := cam.Gain
		if gain <= 0 {
			gain = 3.0
		}
		r.gains[name] = NewGainController(gain)
		if !cam.Enabled {
			continue
		}
		if err := r.register(name, cam); err != nil {
			return nil, err
		}
	}

	return r, nil
}

// NewSpeaker creates a Speaker for the given camera config.
// go2rtcURL and advertiseIP are only used for go2rtc cameras.
func NewSpeaker(cam config.CameraConfig, name, go2rtcURL, advertiseIP string) (Speaker, error) {
	switch cam.Type {
	case "hikvision":
		return NewHikvisionClient(cam.IP, cam.User, cam.Pass, cam.Channel, name), nil
	case "reolink":
		// Reolink two-way audio is not natively implemented yet. If a go2rtc instance
		// is reachable and a stream name is available, route audio through go2rtc instead.
		if go2rtcURL != "" && cam.Stream != "" {
			// Validate that the stream exists in go2rtc to give an early, helpful error
			// instead of a confusing 404 at send time.
			if !Go2rtcStreamExists(go2rtcURL, cam.Stream) {
				available := []string{}
				if streams, err := ListGo2rtcStreams(go2rtcURL); err == nil {
					for s := range streams {
						available = append(available, s)
					}
				}
				newLogger("registry").Warn(
					"reolink go2rtc stream not found — audio will fail",
					"camera", name,
					"stream", cam.Stream,
					"go2rtc", go2rtcURL,
					"available_streams", available,
				)
			}
			newLogger(
				"registry",
			).Info("reolink routing via go2rtc", "camera", name, "stream", cam.Stream, "go2rtc", go2rtcURL)
			return NewGo2rtcClient(go2rtcURL, cam.Stream, cam.IP, advertiseIP, name), nil
		}
		newLogger(
			"registry",
		).Warn("reolink camera has no go2rtc stream; using stub", "camera", name, "go2rtc_url", go2rtcURL, "stream", cam.Stream)
		return NewReolinkClient(cam.IP, cam.User, cam.Pass), nil
	case "go2rtc":
		if go2rtcURL == "" {
			return nil, fmt.Errorf(
				"camera %q uses go2rtc type but CAMSPEAK_GO2RTC_URL is not set",
				name,
			)
		}
		if cam.Stream == "" {
			return nil, fmt.Errorf("camera %q uses go2rtc type but no stream name configured", name)
		}
		return NewGo2rtcClient(go2rtcURL, cam.Stream, cam.IP, advertiseIP, name), nil
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
		return NewOnvifClient(rtspURL, cam.IP, name), nil
	default:
		return nil, fmt.Errorf("unknown camera type %q for camera %q", cam.Type, name)
	}
}

// register creates and registers a Speaker for the given camera config.
func (r *Registry) register(name string, cam config.CameraConfig) error {
	speaker, err := NewSpeaker(cam, name, r.go2rtcURL, r.advertiseIP)
	if err != nil {
		return err
	}
	r.cameras[name] = speaker
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
	// Sync the gain controller if the config gain changed.
	gain := cam.Gain
	if gain <= 0 {
		gain = 3.0
	}
	if gc, ok := r.gains[name]; ok {
		gc.Set(gain)
	} else {
		r.gains[name] = NewGainController(gain)
	}
}

// GetGain returns the GainController for a camera, or nil if not found.
func (r *Registry) GetGain(name string) *GainController {
	return r.gains[name]
}

// SetGain updates the runtime gain for a camera. Takes effect on the next
// audio chunk — no need to restart playback.
func (r *Registry) SetGain(name string, gain float64) {
	if gc, ok := r.gains[name]; ok {
		gc.Set(gain)
	}
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
			return nil, fmt.Errorf(
				"camera %q not registered and on-demand init failed: %w",
				name,
				err,
			)
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

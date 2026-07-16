package cameras

import (
	"fmt"
	"os/exec"

	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/tts"
)

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

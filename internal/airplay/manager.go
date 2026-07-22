package airplay

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"

	clog "github.com/charmbracelet/log"

	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/config"
)

// Manager tracks per-camera shairport-sync instances and supports live enable/disable
// without requiring a server restart.
type Manager struct {
	mu        sync.Mutex
	receivers map[string]Receiver // camera name → running receiver
	ports     map[string]int      // camera name → assigned port (stable across toggles)
	cfg       *config.Config
	reg       *cameras.Registry
	log       *clog.Logger
}

// NewManager creates a Manager, assigns stable RTSP ports to all cameras,
// and starts shairport-sync for cameras where both cam.Enabled and cam.AirPlayEnabled are true.
func NewManager(cfg *config.Config, reg *cameras.Registry) *Manager {
	m := &Manager{
		receivers: make(map[string]Receiver),
		ports:     make(map[string]int),
		cfg:       cfg,
		reg:       reg,
		log: clog.NewWithOptions(os.Stderr, clog.Options{
			Prefix:          "airplay",
			ReportTimestamp: true,
			Level:           clog.InfoLevel,
		}),
	}

	// Assign ports in sorted order so they are deterministic across restarts.
	names := make([]string, 0, len(cfg.Cameras))
	for name := range cfg.Cameras {
		names = append(names, name)
	}
	sort.Strings(names)
	port := cfg.AirPlay.BasePort
	for _, name := range names {
		m.ports[name] = port
		port++
	}

	// Kill any stale shairport-sync processes from a previous unclean exit.
	KillAllStale()

	// Start receivers for cameras that have AirPlay enabled.
	for _, name := range names {
		cam := cfg.Cameras[name]
		if !cam.Enabled || !cam.AirPlayEnabled {
			continue
		}
		if err := m.startLocked(name); err != nil {
			m.log.Warn("AirPlay start failed", "camera", name, "err", err)
		}
	}

	return m
}

// SetLogLevel changes the log level for the manager and all running receivers.
func (m *Manager) SetLogLevel(level clog.Level) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.log.SetLevel(level)
	for _, r := range m.receivers {
		r.SetLogLevel(level)
	}
}

// Enable starts a shairport-sync receiver for the named camera.
// No-op if already running.
func (m *Manager) Enable(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startLocked(name)
}

// Disable stops the shairport-sync receiver for the named camera.
// No-op if not running.
func (m *Manager) Disable(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopLocked(name)
}

// IsRunning reports whether a shairport-sync subprocess is active for the camera.
func (m *Manager) IsRunning(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.receivers[name]
	return ok
}

// Status returns a map of camera name → running for all known cameras.
func (m *Manager) Status() map[string]bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]bool, len(m.ports))
	for name := range m.ports {
		_, out[name] = m.receivers[name]
	}
	return out
}

// Stop shuts down all running receivers (called on server shutdown).
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name := range m.receivers {
		m.stopLocked(name)
	}
}

// startLocked starts a receiver; must hold m.mu.
func (m *Manager) startLocked(name string) error {
	if _, ok := m.receivers[name]; ok {
		return nil // already running
	}
	port, ok := m.ports[name]
	if !ok {
		return fmt.Errorf("no port assigned for camera %q", name)
	}
	camSpeaker, err := m.reg.Get(name)
	if err != nil {
		return fmt.Errorf("getting speaker: %w", err)
	}
	cam := m.cfg.Cameras[name]
	displayName := cam.AirPlayName
	if displayName == "" {
		displayName = cameraDisplayName(name)
	}
	// cameras.Speaker and airplay.Speaker define the same methods; wrap via adapter.
	spk := speakerAdapter{camSpeaker}

	// Prefer shairport-sync when available (handles FairPlay, ALAC natively).
	// Fall back to the built-in pure-Go RAOP receiver (no external deps — good
	// for local development and environments where shairport-sync isn't installed).
	var srv Receiver
	ssp, err := NewShairportServer(displayName, port, m.cfg.AdvertiseIP, spk)
	if err == nil {
		if startErr := ssp.Start(); startErr == nil {
			srv = ssp
		} else {
			m.log.Info("shairport-sync unavailable, using built-in RAOP receiver",
				"camera", name, "err", startErr)
		}
	}
	if srv == nil {
		goSrv, goErr := NewServer(displayName, port, m.cfg.AdvertiseIP, spk)
		if goErr != nil {
			return fmt.Errorf("starting built-in RAOP receiver: %w", goErr)
		}
		goSrv.primeSilenceMs = m.cfg.AirPlay.PrimeSilenceMs
		if startErr := goSrv.Start(); startErr != nil {
			return fmt.Errorf("starting built-in RAOP receiver: %w", startErr)
		}
		srv = goSrv
	}

	srv.SetLogLevel(m.log.GetLevel())
	m.receivers[name] = srv
	m.log.Info("AirPlay receiver started", "camera", name, "port", port, "name", displayName)
	return nil
}

// stopLocked stops a receiver; must hold m.mu.
func (m *Manager) stopLocked(name string) {
	r, ok := m.receivers[name]
	if !ok {
		return
	}
	r.Stop()
	delete(m.receivers, name)
	m.log.Info("AirPlay receiver stopped", "camera", name)
}

// speakerAdapter bridges cameras.Speaker to airplay.Speaker (identical methods,
// different types due to Go's structural typing across packages).
type speakerAdapter struct{ cameras.Speaker }

func (a speakerAdapter) SendRaw(rawFile string) error { return a.Speaker.SendRaw(rawFile) }
func (a speakerAdapter) Stream(r io.Reader) error     { return a.Speaker.Stream(r) }
func (a speakerAdapter) Stop() error                  { return a.Speaker.Stop() }

// cameraDisplayName converts a camera key like "backyard" to "Backyard Camera".
func cameraDisplayName(name string) string {
	if name == "" {
		return name
	}
	words := strings.Fields(strings.ReplaceAll(name, "_", " "))
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ") + " Camera"
}

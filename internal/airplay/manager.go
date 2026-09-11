package airplay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"sort"
	"strings"
	"sync"

	clog "github.com/charmbracelet/log"

	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/logging"
)

// Manager tracks per-camera shairport-sync instances and supports live enable/disable
// without requiring a server restart.
type Manager struct {
	opMu      sync.Mutex // serialize lifecycle changes without blocking status reads
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
	ownedConfig := *cfg
	ownedConfig.Cameras = maps.Clone(cfg.Cameras)
	if ownedConfig.Cameras == nil {
		ownedConfig.Cameras = make(map[string]config.CameraConfig)
	}
	m := &Manager{
		receivers: make(map[string]Receiver),
		ports:     make(map[string]int),
		cfg:       &ownedConfig,
		reg:       reg,
		log:       logging.New("airplay", clog.InfoLevel),
	}

	// Assign ports in sorted order so they are deterministic across restarts.
	names := make([]string, 0, len(cfg.Cameras))
	for name := range cfg.Cameras {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if _, err := m.assignPort(name); err != nil {
			m.log.Warn("AirPlay port allocation failed", "camera", name, "err", err)
		}
	}

	// Start receivers for cameras that have AirPlay enabled.
	for _, name := range names {
		cam := cfg.Cameras[name]
		if !cam.Enabled || !cam.AirPlayEnabled {
			continue
		}
		if err := m.reconcileLocked(name); err != nil {
			m.log.Warn("AirPlay start failed", "camera", name, "err", err)
		}
	}

	return m
}

// SetLogLevel changes the log level for the manager and all running receivers.
func (m *Manager) SetLogLevel(level clog.Level) {
	m.mu.Lock()
	defer m.mu.Unlock()
	logging.SetLevel(m.log, level)
	for _, r := range m.receivers {
		r.SetLogLevel(level)
	}
}

// Enable starts a shairport-sync receiver for the named camera.
// No-op if already running.
func (m *Manager) Enable(name string) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	return m.startLocked(name)
}

// Disable stops the shairport-sync receiver for the named camera.
// No-op if not running.
func (m *Manager) Disable(name string) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.stopLocked(name)
}

// IsRunning reports whether a shairport-sync subprocess is active for the camera.
func (m *Manager) IsRunning(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.receivers[name]
	return r != nil && r.IsRunning()
}

// Status returns a map of camera name → running for all known cameras.
func (m *Manager) Status() map[string]bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]bool, len(m.ports))
	for name := range m.ports {
		r := m.receivers[name]
		out[name] = r != nil && r.IsRunning()
	}
	return out
}

// Stop shuts down all running receivers (called on server shutdown).
func (m *Manager) Stop() {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	for name := range m.receivers {
		m.stopLocked(name)
	}
}

// RestartRunning stops and restarts all currently running receivers so they
// pick up the latest AirPlay config (model, gain, prime silence, etc.).
func (m *Manager) RestartRunning() {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	names := make([]string, 0, len(m.receivers))
	for name := range m.receivers {
		names = append(names, name)
	}
	for _, name := range names {
		m.stopLocked(name)
		if err := m.reconcileLocked(name); err != nil {
			m.log.Warn("AirPlay restart failed", "camera", name, "err", err)
		}
	}
}

// UpdateCamera retires the previous receiver and binds the current speaker and settings.
func (m *Manager) UpdateCamera(name string, cam config.CameraConfig) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.stopLocked(name)
	m.cfg.Cameras[name] = cam
	if !m.cfg.AirPlay.Enabled || !cam.Enabled || !cam.AirPlayEnabled {
		return nil
	}
	return m.reconcileLocked(name)
}

// RemoveCamera stops its receiver and releases its port reservation.
func (m *Manager) RemoveCamera(name string) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.stopLocked(name)
	delete(m.cfg.Cameras, name)
	m.mu.Lock()
	delete(m.ports, name)
	m.mu.Unlock()
}

// UpdateConfig applies global receiver settings, restarting eligible cameras.
func (m *Manager) UpdateConfig(cfg config.AirPlayConfig) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if cfg.BasePort < 1 || cfg.BasePort > 65535 {
		return fmt.Errorf("AirPlay base port must be between 1 and 65535")
	}
	for name := range m.receivers {
		m.stopLocked(name)
	}
	if cfg.BasePort != m.cfg.AirPlay.BasePort {
		m.mu.Lock()
		clear(m.ports)
		m.mu.Unlock()
	}
	m.cfg.AirPlay = cfg
	if !cfg.Enabled {
		return nil
	}
	names := make([]string, 0, len(m.cfg.Cameras))
	for name := range m.cfg.Cameras {
		names = append(names, name)
	}
	sort.Strings(names)
	var errs []error
	for _, name := range names {
		cam := m.cfg.Cameras[name]
		if cam.Enabled && cam.AirPlayEnabled {
			if err := m.reconcileLocked(name); err != nil {
				errs = append(errs, fmt.Errorf("camera %q: %w", name, err))
			}
		}
	}
	return errors.Join(errs...)
}

// UpdateRouting restarts receivers to advertise the current address.
func (m *Manager) UpdateRouting(advertiseIP string) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	m.cfg.AdvertiseIP = advertiseIP
	var errs []error
	names := make([]string, 0, len(m.receivers))
	for name := range m.receivers {
		names = append(names, name)
	}
	for _, name := range names {
		m.stopLocked(name)
		if err := m.reconcileLocked(name); err != nil {
			errs = append(errs, fmt.Errorf("camera %q: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

// reconcileLocked keeps unsupported receivers absent without making ordinary
// camera/global configuration saves fail. Explicit Enable still reports why.
func (m *Manager) reconcileLocked(name string) error {
	err := m.startLocked(name)
	if errors.Is(err, cameras.ErrLiveStreamUnsupported) {
		m.log.Info("AirPlay unavailable for camera backend", "camera", name)
		return nil
	}
	return err
}

// assignPort reserves a free stable port for a newly added camera.
// The operating system checks external collisions when the receiver binds.
func (m *Manager) assignPort(name string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if port, ok := m.ports[name]; ok {
		return port, nil
	}
	used := make(map[int]bool, len(m.ports))
	for _, port := range m.ports {
		used[port] = true
	}
	for port := m.cfg.AirPlay.BasePort; port > 0 && port <= 65535; port++ {
		if !used[port] {
			m.ports[name] = port
			return port, nil
		}
	}
	return 0, fmt.Errorf("no AirPlay port available for camera %q", name)
}

// startLocked starts a receiver; lifecycle callers hold m.opMu.
func (m *Manager) startLocked(name string) error {
	if _, ok := m.receivers[name]; ok {
		return nil // already running
	}
	cam, ok := m.cfg.Cameras[name]
	if !ok {
		return fmt.Errorf("unknown camera %q", name)
	}
	if !m.cfg.AirPlay.Enabled || !cam.Enabled || !cam.AirPlayEnabled {
		return nil
	}
	camSpeaker, err := m.reg.Get(name)
	if err != nil {
		return fmt.Errorf("getting speaker: %w", err)
	}
	if !cameras.CanLiveStream(camSpeaker) {
		return fmt.Errorf("camera %q: %w", name, cameras.ErrLiveStreamUnsupported)
	}
	port, err := m.assignPort(name)
	if err != nil {
		return err
	}
	displayName := cam.AirPlayName
	if displayName == "" {
		displayName = cameraDisplayName(name)
	}
	// cameras.Speaker and airplay.Speaker define the same methods; wrap via adapter.
	spk := speakerAdapter{camSpeaker}

	model := cam.AirPlayModel
	if model == "" {
		model = m.cfg.AirPlay.Model
	}
	gain := cam.Gain

	// Prefer shairport-sync when available (handles FairPlay, ALAC natively).
	// Fall back to the built-in pure-Go RAOP receiver (no external deps — good
	// for local development and environments where shairport-sync isn't installed).
	var srv Receiver
	var backend string
	ssp, err := NewShairportServer(displayName, port, m.cfg.AdvertiseIP, spk, model, gain)
	if err != nil {
		m.log.Debug("shairport-sync setup failed, falling back", "camera", name, "err", err)
	}
	if err == nil {
		ssp.primeSilenceMs = m.cfg.AirPlay.PrimeSilenceMs
		if startErr := ssp.Start(); startErr == nil {
			srv = ssp
			backend = "shairport-sync"
		} else {
			m.log.Info("shairport-sync unavailable, using built-in RAOP receiver",
				"camera", name, "err", startErr)
		}
	}
	if srv == nil {
		goSrv, goErr := NewServer(displayName, port, m.cfg.AdvertiseIP, spk, model, gain)
		if goErr != nil {
			return fmt.Errorf("starting built-in RAOP receiver: %w", goErr)
		}
		goSrv.primeSilenceMs = m.cfg.AirPlay.PrimeSilenceMs
		if startErr := goSrv.Start(); startErr != nil {
			return fmt.Errorf("starting built-in RAOP receiver: %w", startErr)
		}
		srv = goSrv
		backend = "built-in-raop"
	}

	srv.SetLogLevel(m.log.GetLevel())
	m.mu.Lock()
	m.receivers[name] = srv
	m.mu.Unlock()
	m.log.Info(
		"AirPlay receiver started",
		"camera",
		name,
		"port",
		port,
		"name",
		displayName,
		"backend",
		backend,
	)
	return nil
}

// stopLocked removes state before performing I/O; callers hold m.opMu.
func (m *Manager) stopLocked(name string) {
	m.mu.Lock()
	r, ok := m.receivers[name]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(m.receivers, name)
	m.mu.Unlock()
	r.Stop()
	m.log.Info("AirPlay receiver stopped", "camera", name)
}

// speakerAdapter bridges cameras.Speaker to airplay.Speaker (identical methods,
// different types due to Go's structural typing across packages).
type speakerAdapter struct{ cameras.Speaker }

func (a speakerAdapter) SendRaw(rawFile string) (SendTiming, error) {
	t, err := a.Speaker.SendRaw(rawFile, nil)
	return SendTiming{OpenMs: t.OpenMs, PlaybackMs: t.PlaybackMs}, err
}
func (a speakerAdapter) Stream(r io.Reader) error { return a.Speaker.Stream(r) }
func (a speakerAdapter) Stop() error              { return a.Speaker.Stop() }

func (a speakerAdapter) AirPlayState() (uint64, bool) {
	return a.Speaker.(cameras.PrioritySpeaker).AudioPriority().State()
}

func (a speakerAdapter) BeginAirPlay(
	ctx context.Context,
	generation uint64,
) (context.Context, func(), bool) {
	return a.Speaker.(cameras.PrioritySpeaker).AudioPriority().BeginAirPlay(ctx, generation)
}

func (a speakerAdapter) StreamContext(ctx context.Context, r io.Reader) error {
	return cameras.StreamContext(ctx, a.Speaker, r)
}

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

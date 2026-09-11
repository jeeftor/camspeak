package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"

	"github.com/jeeftor/camspeak/internal/airplay"
	"github.com/jeeftor/camspeak/internal/api"
	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/discovery"
	"github.com/jeeftor/camspeak/internal/frigate"
	"github.com/jeeftor/camspeak/internal/library"
	"github.com/jeeftor/camspeak/internal/tts"
	"github.com/jeeftor/camspeak/internal/util"
	"github.com/jeeftor/camspeak/internal/vision"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the camspeak HTTP server",
	RunE:  runServe,
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	appLog.Info("starting camspeak", "version", version)

	// Propagate log level to sub-packages
	level := logLevel()
	api.SetLogLevel(level)
	cameras.SetLogLevel(level)
	discovery.SetLogLevel(level)
	tts.SetLogLevel(level)
	vision.SetLogLevel(level)

	database, dir, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	cfg, err := config.Load(database)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Override library path to be within data dir if using default
	if cfg.Library == "/config/library" {
		cfg.Library = filepath.Join(dir, "library")
	}

	// Auto-discover cameras from Frigate if configured and no cameras in DB
	if cfg.FrigateURL != "" && len(cfg.Cameras) == 0 {
		appLog.Info("discovering cameras from Frigate", "url", cfg.FrigateURL)
		discoverer := frigate.NewDiscoverer(cfg.FrigateURL)
		cams, discoverErr := discoverer.Discover()
		if discoverErr != nil {
			appLog.Warn("Frigate discovery failed", "err", discoverErr)
		} else if len(cams) > 0 {
			if saveErr := frigate.SaveToDB(database, cams); saveErr != nil {
				appLog.Warn("saving discovered cameras", "err", saveErr)
			} else {
				appLog.Info("discovered cameras", "count", len(cams))
				// Reload config to pick up the new cameras
				cfg, err = config.Load(database)
				if err != nil {
					return fmt.Errorf("reloading config: %w", err)
				}
				cfg.Library = filepath.Join(dir, "library")
			}
		}
	}

	printBanner(cfg)

	if !cameras.FFmpegAvailable() {
		appLog.Warn("ffmpeg not found — audio transcoding will fail")
	}

	ttsClient := tts.NewClient(cfg.TTS.URL, cfg.TTS.Model, cfg.TTS.APIKey)

	tmpDir := filepath.Join(dir, "tmp")
	store, err := library.NewStore(cfg.Library, tmpDir)
	if err != nil {
		return fmt.Errorf("library: %w", err)
	}

	reg, err := cameras.NewRegistry(cfg, ttsClient)
	if err != nil {
		return fmt.Errorf("camera registry: %w", err)
	}

	appLog.Info("cameras loaded", "count", len(cfg.Cameras))

	for name, cam := range cfg.Cameras {
		appLog.Info("camera", "name", name, "type", cam.Type, "ip", cam.IP, "enabled", cam.Enabled)
	}

	// Start AirPlay receivers via the Manager (shairport-sync backend).
	// The Manager assigns stable ports and handles per-camera enable/disable live.
	airplayMgr := airplay.NewManager(cfg, reg)
	airplayMgr.SetLogLevel(level)
	if cfg.AirPlay.Enabled {
		appLog.Info("AirPlay enabled — starting Manager")
	}

	srv := api.New(cfg, reg, store, ttsClient, database)

	// Wire AirPlay Manager → API so per-camera toggles take effect live.
	if airplayMgr != nil {
		srv.Handlers().SetAirPlayManager(airplayMgr)
	}

	// Advertise via mDNS so Home Assistant can auto-discover camspeak.
	var disc *discovery.Service
	if d, err := discovery.Register(cfg.Port, version, len(cfg.Cameras), cfg.AdvertiseIP); err != nil {
		appLog.Warn("mDNS discovery failed (non-fatal)", "err", err)
	} else {
		disc = d
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	serverReturned := make(chan struct{})
	shutdownDone := make(chan struct{})

	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	go func() {
		defer close(shutdownDone)
		select {
		case <-quit:
		case <-serverReturned:
		}
		appLog.Info("shutting down")
		if disc != nil {
			disc.Shutdown()
		}
		if airplayMgr != nil {
			airplayMgr.Stop()
		}
		srv.Stop() //nolint:errcheck
	}()

	addr := fmt.Sprintf(":%d", cfg.Port)

	err = srv.Start(addr)
	close(serverReturned)
	// Keep SQLite open until uploads, playback and receiver shutdown finish.
	<-shutdownDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

var infoStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#626262"))

func versionBanner(value string, color bool) string {
	renderer := lipgloss.NewRenderer(os.Stdout)
	renderer.SetColorProfile(termenv.Ascii)
	if color {
		// Docker captures stdout without a TTY; color only this startup marker.
		renderer.SetColorProfile(termenv.ANSI)
	}
	badge := renderer.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
	return "  camspeak  " + badge.Render("[ VERSION "+value+" ]") + "\n"
}

func printBanner(cfg *config.Config) {
	fmt.Println(versionBanner(version, os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"))
	fmt.Println(infoStyle.Render(fmt.Sprintf("  UI  -> http://localhost:%d", cfg.Port)))
	fmt.Println(infoStyle.Render(fmt.Sprintf("  MCP -> http://localhost:%d/mcp", cfg.Port)))
	fmt.Println(infoStyle.Render("  TTS -> " + util.RedactURLString(cfg.TTS.URL)))
	if cfg.AirPlay.Enabled {
		fmt.Println(
			infoStyle.Render(
				fmt.Sprintf("  AirPlay -> enabled (base port %d)", cfg.AirPlay.BasePort),
			),
		)
	}
	fmt.Println()
}

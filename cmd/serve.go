package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/jeeftor/camspeak/internal/airplay"
	"github.com/jeeftor/camspeak/internal/api"
	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/frigate"
	"github.com/jeeftor/camspeak/internal/library"
	"github.com/jeeftor/camspeak/internal/mqtt"
	"github.com/jeeftor/camspeak/internal/tts"
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

	ttsClient := tts.NewClient(cfg.TTS.URL, cfg.TTS.Model)

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
	var airplayMgr *airplay.Manager
	if cfg.AirPlay.Enabled {
		appLog.Info("AirPlay enabled — starting Manager")
		airplayMgr = airplay.NewManager(cfg, reg)
		airplayMgr.SetLogLevel(level)
	}

	srv := api.New(cfg, reg, store, ttsClient, database)

	// Wire AirPlay Manager → API so per-camera toggles take effect live.
	if airplayMgr != nil {
		srv.Handlers().SetAirPlayManager(airplayMgr)
	}

	// Wire MQTT → API handlers
	mqttSub := mqtt.New(cfg.MQTT, cfg.Rules, srv.Handlers().SpeakForMQTT)
	mqttSub.SetMessageHook(srv.Handlers().HandleMQTTMessage)
	srv.Handlers().SetMQTT(cfg.MQTT.Broker, mqttSub.Status)
	srv.Handlers().SetMQTTSubscribeFn(mqttSub.SubscribeTopic)
	if err := mqttSub.Start(); err != nil {
		appLog.Warn("MQTT not connected", "err", err)
	}
	defer mqttSub.Stop()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)

	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		appLog.Info("shutting down")
		if airplayMgr != nil {
			airplayMgr.Stop()
		}
		srv.Stop() //nolint:errcheck
	}()

	addr := fmt.Sprintf(":%d", cfg.Port)

	return srv.Start(addr)
}

var titleStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("#7D56F4")).
	MarginBottom(1)

var infoStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#626262"))

func printBanner(cfg *config.Config) {
	fmt.Println(titleStyle.Render(fmt.Sprintf("  camspeak %s", version)))
	fmt.Println(infoStyle.Render(fmt.Sprintf("  UI  -> http://localhost:%d", cfg.Port)))
	fmt.Println(infoStyle.Render(fmt.Sprintf("  MCP -> http://localhost:%d/mcp", cfg.Port)))
	fmt.Println(infoStyle.Render("  TTS -> " + cfg.TTS.URL))
	if cfg.AirPlay.Enabled {
		fmt.Println(
			infoStyle.Render(
				fmt.Sprintf("  AirPlay -> enabled (base port %d)", cfg.AirPlay.BasePort),
			),
		)
	}
	fmt.Println()
}

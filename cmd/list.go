package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/library"
	"github.com/jeeftor/camspeak/internal/tts"
)

var listCmd = &cobra.Command{
	Use:   "list [cameras|presets]",
	Short: "List cameras or presets",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, dir, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		cfg, err := config.Load(database)
		if err != nil {
			return err
		}

		switch args[0] {
		case "cameras":
			ttsClient := tts.NewClient(cfg.TTS.URL, cfg.TTS.Model)

			reg, err := cameras.NewRegistry(cfg, ttsClient)
			if err != nil {
				return err
			}

			status := reg.Status()
			for _, name := range reg.Names() {
				online := "offline"
				if status[name] {
					online = "online"
				}

				fmt.Printf("  %-20s %s (%s)\n", name, online, cfg.Cameras[name].Type)
			}
		case "presets":
			libDir := cfg.Library
			if libDir == "/config/library" {
				libDir = filepath.Join(dir, "library")
			}
			store, err := library.NewStore(libDir)
			if err != nil {
				return err
			}

			presets, err := store.List()
			if err != nil {
				return err
			}

			if len(presets) == 0 {
				fmt.Println("  (no presets)")

				return nil
			}

			for _, p := range presets {
				fmt.Printf("  %-20s %-12s %.1fs  %q\n", p.Name, p.Category, p.Duration, p.Text)
			}
		default:
			return fmt.Errorf("unknown resource %q (use: cameras, presets)", args[0])
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}

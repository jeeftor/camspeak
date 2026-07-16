package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jeeftor/camspeak/internal/api"
	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/tts"
)

var beepCmd = &cobra.Command{
	Use:   "beep <camera>",
	Short: "Play a test beep on a camera",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, _, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		cfg, err := config.Load(database)
		if err != nil {
			return err
		}

		ttsClient := tts.NewClient(cfg.TTS.URL, cfg.TTS.Model)

		reg, err := cameras.NewRegistry(cfg, ttsClient)
		if err != nil {
			return err
		}

		cam, err := reg.Get(args[0])
		if err != nil {
			return err
		}

		raw, err := api.GenerateBeep("")
		if err != nil {
			return err
		}

		appLog.Info("beeping", "camera", args[0])

		if err := cam.SendRaw(raw); err != nil {
			return fmt.Errorf("camera: %w", err)
		}

		appLog.Info("done")

		return nil
	},
}

func init() {
	rootCmd.AddCommand(beepCmd)
}

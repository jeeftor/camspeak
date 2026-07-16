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

var speakVoice string

var speakCmd = &cobra.Command{
	Use:   "speak <camera> <text>",
	Short: "Send TTS to a camera speaker",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		cameraName, text := args[0], args[1]

		database, dir, err := openDB()
		if err != nil {
			return err
		}
		defer database.Close()

		cfg, err := config.Load(database)
		if err != nil {
			return err
		}

		voice := speakVoice
		if voice == "" {
			voice = cfg.TTS.DefaultVoice
		}

		ttsClient := tts.NewClient(cfg.TTS.URL, cfg.TTS.Model)

		libDir := cfg.Library
		if libDir == "/config/library" {
			libDir = filepath.Join(dir, "library")
		}
		store, err := library.NewStore(libDir)
		if err != nil {
			return err
		}

		reg, err := cameras.NewRegistry(cfg, ttsClient)
		if err != nil {
			return err
		}

		cam, err := reg.Get(cameraName)
		if err != nil {
			return err
		}

		appLog.Info("generating TTS", "camera", cameraName, "text", text, "voice", voice)

		wav, err := ttsClient.Speak(text, voice)
		if err != nil {
			return fmt.Errorf("TTS: %w", err)
		}

		preset, err := store.Save("_tmp", "cli_speak", text, voice, wav)
		if err != nil {
			return fmt.Errorf("transcoding: %w", err)
		}

		appLog.Info(
			"sending audio",
			"camera",
			cameraName,
			"duration",
			fmt.Sprintf("%.1fs", preset.Duration),
		)

		if err := cam.SendRaw(preset.RawPath); err != nil {
			return fmt.Errorf("camera: %w", err)
		}

		appLog.Info("done")

		return nil
	},
}

func init() {
	rootCmd.AddCommand(speakCmd)
	speakCmd.Flags().StringVarP(&speakVoice, "voice", "v", "", "TTS voice (default: from config)")
}

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/frigate"
)

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Discover cameras from Frigate NVR",
	Long: `Discover cameras from a Frigate NVR instance by parsing its config.
Requires CAMSPEAK_FRIGATE_URL env var or --frigate flag.
Discovered cameras are saved to the SQLite database.`,
	RunE: runDiscover,
}

var discoverFrigateURL string

func init() {
	rootCmd.AddCommand(discoverCmd)
	discoverCmd.Flags().
		StringVar(&discoverFrigateURL, "frigate", "", "Frigate URL (default: from config/env)")
}

func runDiscover(cmd *cobra.Command, args []string) error {
	database, _, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	// Determine Frigate URL
	frigateURL := discoverFrigateURL
	if frigateURL == "" {
		cfg, err := config.Load(database)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		frigateURL = cfg.FrigateURL
	}
	if frigateURL == "" {
		return fmt.Errorf("no Frigate URL configured — set CAMSPEAK_FRIGATE_URL or use --frigate")
	}

	appLog.Info("discovering cameras from Frigate", "url", frigateURL)

	discoverer := frigate.NewDiscoverer(frigateURL)
	cams, err := discoverer.Discover()
	if err != nil {
		return fmt.Errorf("discovery failed: %w", err)
	}

	if len(cams) == 0 {
		appLog.Warn("no cameras found")
		return nil
	}

	// Save to database
	if err := frigate.SaveToDB(database, cams); err != nil {
		return fmt.Errorf("saving cameras: %w", err)
	}

	appLog.Info("discovered and saved cameras", "count", len(cams))
	fmt.Println()

	for _, cam := range cams {
		fmt.Printf("  %-20s %s  %s\n", cam.Name, cam.Type, cam.IP)
	}

	return nil
}

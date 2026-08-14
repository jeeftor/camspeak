package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/jeeftor/camspeak/internal/api"
	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/config"
)

var (
	tgGo2rtcURL string
	tgStream    string
	tgBeep      bool
)

var testGo2rtcCmd = &cobra.Command{
	Use:   "test-go2rtc",
	Short: "List go2rtc streams and test audio routing to a camera",
	Long: `Lists all streams configured in go2rtc and optionally sends a test beep
to a specific stream to verify the audio backchannel works.

If no --go2rtc-url is given, uses CAMSPEAK_GO2RTC_URL from the database or env.
If --stream is given, sends a test beep to that stream.

Examples:
  camspeak test-go2rtc
  camspeak test-go2rtc --go2rtc-url http://192.168.1.120:1984
  camspeak test-go2rtc --stream reolink_doorbell
  camspeak test-go2rtc --stream reolink_doorbell --beep
`,
	RunE: runTestGo2rtc,
}

func init() {
	rootCmd.AddCommand(testGo2rtcCmd)
	testGo2rtcCmd.Flags().
		StringVar(&tgGo2rtcURL, "go2rtc-url", "", "go2rtc URL (default: from config/env)")
	testGo2rtcCmd.Flags().StringVar(&tgStream, "stream", "", "stream name to test (sends a beep)")
	testGo2rtcCmd.Flags().BoolVar(&tgBeep, "beep", true, "send a test beep to the stream")
}

var (
	tgStyleOK   = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	tgStyleErr  = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	tgStyleWarn = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	tgStyleDim  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	tgStyleBold = lipgloss.NewStyle().Bold(true)
	tgStyleName = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	tgStyleSrc  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	tgStyleHdr  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
)

func runTestGo2rtc(cmd *cobra.Command, args []string) error {
	// Resolve go2rtc URL
	go2rtcURL := tgGo2rtcURL
	if go2rtcURL == "" {
		database, _, err := openDB()
		if err == nil {
			cfg, _ := config.Load(database)
			database.Close()
			go2rtcURL = cfg.Go2rtcURL
			if go2rtcURL == "" {
				go2rtcURL = cameras.FindGo2rtcURL(cfg.FrigateURL)
			}
		}
	}
	if go2rtcURL == "" {
		if v := os.Getenv("CAMSPEAK_GO2RTC_URL"); v != "" {
			go2rtcURL = v
		}
	}
	if go2rtcURL == "" {
		fmt.Println()
		fmt.Println(tgStyleErr.Render("  ✗ No go2rtc URL found"))
		fmt.Println(
			tgStyleDim.Render(
				"    Set --go2rtc-url, CAMSPEAK_GO2RTC_URL, or configure it in Config → Settings",
			),
		)
		fmt.Println()
		return fmt.Errorf("no go2rtc URL configured")
	}

	fmt.Println()
	fmt.Println(tgStyleBold.Render("  camspeak go2rtc test"))
	fmt.Printf("  go2rtc: %s\n\n", tgStyleDim.Render(go2rtcURL))

	// Step 1: Check go2rtc is reachable
	fmt.Print("  [1/3] Checking go2rtc API... ")
	streams, err := cameras.ListGo2rtcStreams(go2rtcURL)
	if err != nil {
		fmt.Println(tgStyleErr.Render("FAILED"))
		fmt.Printf("  %s %v\n\n", tgStyleErr.Render("ERROR:"), err)
		return err
	}
	fmt.Println(
		tgStyleOK.Render("OK") + tgStyleDim.Render(fmt.Sprintf("  (%d streams)", len(streams))),
	)

	// Step 2: List streams
	fmt.Println()
	fmt.Println(tgStyleHdr.Render("  Streams:"))
	if len(streams) == 0 {
		fmt.Println(tgStyleWarn.Render("    (no streams configured in go2rtc)"))
	} else {
		names := make([]string, 0, len(streams))
		for name := range streams {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			src := streams[name]
			// Highlight backchannel streams
			marker := " "
			if strings.Contains(src, "backchannel") {
				marker = tgStyleOK.Render("✓")
			}
			fmt.Printf("    %s %s  %s\n", marker, tgStyleName.Render(name), tgStyleSrc.Render(src))
		}
	}
	fmt.Println()

	// Step 3: Test beep if stream specified
	if tgStream != "" {
		fmt.Printf("  [2/3] Stream %s... ", tgStyleName.Render(tgStream))
		src, exists := streams[tgStream]
		if !exists {
			fmt.Println(tgStyleErr.Render("NOT FOUND"))
			fmt.Printf("  %s Stream %q does not exist in go2rtc\n",
				tgStyleErr.Render("ERROR:"), tgStream)
			fmt.Println(tgStyleDim.Render("  Available streams listed above."))
			fmt.Println()
			fmt.Println(
				tgStyleDim.Render(
					"  Tip: In Frigate/go2rtc config, add a stream with the same name as your camera,",
				),
			)
			fmt.Println(
				tgStyleDim.Render(
					"       e.g. go2rtc: streams: reolink: rtsp://user:pass@192.168.1.128:554/stream_1#backchannel=1",
				),
			)
			fmt.Println()
			return fmt.Errorf("stream %q not found in go2rtc", tgStream)
		}
		fmt.Println(tgStyleOK.Render("FOUND"))
		if !strings.Contains(src, "backchannel") {
			fmt.Printf("  %s stream source does not contain #backchannel=1\n",
				tgStyleWarn.Render("WARN:"))
			fmt.Println(
				tgStyleDim.Render(
					"        Audio may not work without a backchannel-enabled stream",
				),
			)
		}

		if tgBeep {
			fmt.Print("  [3/3] Sending test beep... ")
			// Generate a beep
			raw, err := api.GenerateBeep("")
			if err != nil {
				fmt.Println(tgStyleErr.Render("FAILED"))
				return fmt.Errorf("generating beep: %w", err)
			}

			// Create a go2rtc client and send
			advertiseIP := os.Getenv("CAMSPEAK_ADVERTISE_IP")
			client := cameras.NewGo2rtcClient(go2rtcURL, tgStream, "", advertiseIP, "test-go2rtc")
			if _, err := client.SendRaw(raw, nil); err != nil {
				fmt.Println(tgStyleErr.Render("FAILED"))
				fmt.Printf("  %s %v\n\n", tgStyleErr.Render("ERROR:"), err)
				return err
			}
			fmt.Println(tgStyleOK.Render("OK"))
			fmt.Println()
			fmt.Println(
				tgStyleBold.Render(
					"  ✓ Beep sent successfully! Check the camera speaker for audio.",
				),
			)
		}
	} else {
		fmt.Println(tgStyleDim.Render("  (use --stream <name> to test sending a beep to a specific stream)"))
	}
	fmt.Println()
	return nil
}

package cmd

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	clog "github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	"github.com/jeeftor/camspeak/internal/airplay"
	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/logging"
)

var airtestCmd = &cobra.Command{
	Use:   "airtest [camera]",
	Short: "Start a one-off AirPlay receiver that buffers audio and sends it to a camera",
	Long: `airtest starts a pure-Go AirPlay/RAOP receiver for a single camera.

Stream audio from an iPhone, then stop AirPlay. The tool buffers the received
G.711ulaw audio and calls the camera's SendRaw endpoint.

If a camera name is given, airtest loads it from the SQLite database (with .env
overrides). Without a name, it creates an ad-hoc camera from environment variables
or flags.

Examples:

  camspeak airtest backyard
  camspeak airtest backyard --user admin --pass secret --port 5100
  camspeak airtest --no-send --output /tmp/captured.raw

Environment variables:
  TEST_CAM_IP, TEST_CAM_USER, TEST_CAM_PASS, TEST_CAM_TYPE, TEST_CAM_CHANNEL,
  TEST_CAM_STREAM, TEST_CAM_NAME  — ad-hoc camera or override values
  CAMSPEAK_AIRTEST_*                — same as TEST_CAM_* (preferred prefix)
  CAMSPEAK_LOG_LEVEL                — debug, info, warn, error
  CAMSPEAK_ADVERTISE_IP             — IP advertised over mDNS`,
	Args: cobra.RangeArgs(0, 1),
	RunE: runAirtest,
}

var (
	airtestIP              string
	airtestUser            string
	airtestPass            string
	airtestType            string
	airtestChannel         int
	airtestONVIFStream     string
	airtestPort            int
	airtestName            string
	airtestAdvertiseIPFlag string
	airtestOutput          string
	airtestNoSend          bool
	airtestDebug           bool
)

func init() {
	rootCmd.AddCommand(airtestCmd)

	airtestCmd.Flags().StringVar(&airtestIP, "ip", "", "Camera IP address")
	airtestCmd.Flags().StringVar(&airtestUser, "user", "", "Camera username")
	airtestCmd.Flags().StringVar(&airtestPass, "pass", "", "Camera password")
	airtestCmd.Flags().StringVar(&airtestType, "type", "", "Camera type: hikvision, onvif (defaults to hikvision)")
	airtestCmd.Flags().IntVar(&airtestChannel, "channel", -1, "Hikvision audio channel (defaults to 1)")
	airtestCmd.Flags().StringVar(&airtestONVIFStream, "onvif-stream", "", "ONVIF RTSP stream URL")

	airtestCmd.Flags().IntVar(&airtestPort, "port", 5100, "AirPlay RTSP listener port")
	airtestCmd.Flags().StringVar(&airtestName, "name", "", "Display name in AirPlay picker")
	airtestCmd.Flags().StringVar(&airtestAdvertiseIPFlag, "advertise-ip", "", "IP to advertise over mDNS (defaults to CAMSPEAK_ADVERTISE_IP or auto-detect)")
	airtestCmd.Flags().StringVar(&airtestOutput, "output", "", "Save received raw audio to this path")
	airtestCmd.Flags().BoolVar(&airtestNoSend, "no-send", false, "Capture audio only; do not send to camera")
	airtestCmd.Flags().BoolVar(&airtestDebug, "debug", false, "Enable debug logging")
}

func runAirtest(cmd *cobra.Command, args []string) error {
	database, _, err := openDB()
	if err != nil {
		return err
	}
	defer database.Close()

	cfg, err := config.Load(database)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	level := logLevel()
	if airtestDebug {
		level = clog.DebugLevel
	}
	log := logging.New("airtest", level)

	var camName string
	if len(args) > 0 {
		camName = args[0]
	}

	cam := buildAirtestCamera(cfg, camName, log)

	if cam.IP == "" {
		return fmt.Errorf("camera IP is required (set TEST_CAM_IP / CAMSPEAK_AIRTEST_IP, -ip, or provide a camera with an IP)")
	}

	speaker, err := buildAirtestSpeaker(cam, log)
	if err != nil {
		return err
	}

	displayName := airtestDisplayName(camName, cam)
	advertiseIP := resolveAirtestAdvertiseIP(cfg)

	server, err := airplay.NewServer(displayName, airtestPort, advertiseIP, speaker)
	if err != nil {
		return fmt.Errorf("creating AirPlay server: %w", err)
	}
	server.SetLogLevel(level)

	if err := server.Start(); err != nil {
		return fmt.Errorf("starting AirPlay server: %w", err)
	}

	log.Info("AirPlay test receiver running",
		"name", displayName,
		"port", airtestPort,
		"advertise_ip", advertiseIP,
		"camera_name", camName,
		"camera_ip", cam.IP,
		"camera_type", cam.Type,
		"no_send", airtestNoSend,
		"output", airtestOutput,
	)
	log.Info("Select the AirPlay target on your iPhone, play audio, then stop AirPlay.")

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Info("shutting down")
	server.Stop()
	return nil
}

// buildAirtestCamera returns the camera config to test, merging database,
// environment, and flag values.
func buildAirtestCamera(cfg *config.Config, name string, log *clog.Logger) config.CameraConfig {
	var cam config.CameraConfig
	if name != "" {
		if existing, ok := cfg.Cameras[name]; ok {
			cam = existing
		} else {
			log.Warn("camera not found in database, using ad-hoc config", "camera", name)
		}
	}

	if v := airtestEnv("TYPE"); v != "" {
		cam.Type = v
	}
	if v := airtestEnv("IP"); v != "" {
		cam.IP = v
	}
	if v := airtestEnv("USER"); v != "" {
		cam.User = v
	}
	if v := airtestEnv("PASS"); v != "" {
		cam.Pass = v
	}
	if v := airtestEnv("CHANNEL"); v != "" {
		if ch, err := strconv.Atoi(v); err == nil {
			cam.Channel = ch
		}
	}
	if v := airtestEnv("STREAM"); v != "" {
		cam.Stream = v
	}

	// Flags override env.
	if airtestType != "" {
		cam.Type = airtestType
	}
	if airtestIP != "" {
		cam.IP = airtestIP
	}
	if airtestUser != "" {
		cam.User = airtestUser
	}
	if airtestPass != "" {
		cam.Pass = airtestPass
	}
	if airtestChannel >= 0 {
		cam.Channel = airtestChannel
	}
	if airtestONVIFStream != "" {
		cam.Stream = airtestONVIFStream
	}

	// Apply sensible defaults if still empty.
	if cam.Type == "" {
		cam.Type = "hikvision"
	}
	if cam.Channel == 0 {
		cam.Channel = 1
	}

	return cam
}

// airtestDisplayName picks the AirPlay display name.
func airtestDisplayName(camName string, cam config.CameraConfig) string {
	if airtestName != "" {
		return airtestName
	}
	if v := airtestEnv("NAME"); v != "" {
		return v
	}
	if camName != "" {
		return fmt.Sprintf("%s (airtest)", camName)
	}
	if cam.IP != "" {
		return fmt.Sprintf("AirPlay Test @ %s", cam.IP)
	}
	return "AirPlay Test"
}

// resolveAirtestAdvertiseIP returns the IP to advertise, preferring the flag,
// then the global config/env, then auto-detection.
func resolveAirtestAdvertiseIP(cfg *config.Config) string {
	if airtestAdvertiseIPFlag != "" {
		return airtestAdvertiseIPFlag
	}
	if cfg.AdvertiseIP != "" {
		return cfg.AdvertiseIP
	}
	return firstNonLoopbackIP()
}

// buildAirtestSpeaker creates a camera speaker wrapped in a buffer that captures
// the received audio before sending it.
func buildAirtestSpeaker(cam config.CameraConfig, log *clog.Logger) (airplay.Speaker, error) {
	var camSpeaker cameras.Speaker
	switch cam.Type {
	case "hikvision":
		camSpeaker = cameras.NewHikvisionClient(cam.IP, cam.User, cam.Pass, cam.Channel, "airtest")
	case "onvif":
		rtspURL := cam.Stream
		if rtspURL == "" {
			if cam.User != "" && cam.Pass != "" {
				rtspURL = fmt.Sprintf("rtsp://%s:%s@%s:554/stream0", cam.User, cam.Pass, cam.IP)
			} else {
				rtspURL = fmt.Sprintf("rtsp://%s:554/stream0", cam.IP)
			}
		}
		camSpeaker = cameras.NewOnvifClient(rtspURL, cam.IP, "airtest")
	case "go2rtc":
		return nil, fmt.Errorf("go2rtc cameras are not supported by airtest; use the full server instead")
	case "reolink":
		return nil, fmt.Errorf("reolink audio is not yet implemented")
	default:
		return nil, fmt.Errorf("unknown camera type %q", cam.Type)
	}

	return &airtestBufferedSpeaker{
		cam:    camSpeaker,
		log:    log.With("component", "buffer"),
		output: airtestOutput,
		noSend: airtestNoSend,
	}, nil
}

// airtestBufferedSpeaker implements airplay.Speaker by buffering the received
// G.711ulaw stream to a file, then sending it to the camera.
type airtestBufferedSpeaker struct {
	cam    cameras.Speaker
	log    *clog.Logger
	output string
	noSend bool
}

func (b *airtestBufferedSpeaker) SendRaw(rawFile string) error {
	b.log.Info("SendRaw called", "file", rawFile)
	if b.noSend {
		return nil
	}
	return b.cam.SendRaw(rawFile)
}

func (b *airtestBufferedSpeaker) Stream(r io.Reader) error {
	path, size, err := b.bufferStream(r)
	if err != nil {
		return err
	}

	if path == "" {
		return nil
	}
	if size == 0 {
		b.log.Warn("no audio received")
		return nil
	}

	b.log.Info("AirPlay audio buffered", "file", path, "bytes", size)

	if b.noSend {
		b.log.Info("-no-send set, skipping camera send")
		return nil
	}

	b.log.Info("Sending buffered audio to camera", "file", path)
	if err := b.cam.SendRaw(path); err != nil {
		return fmt.Errorf("sending to camera: %w", err)
	}
	b.log.Info("Buffered audio sent to camera")
	return nil
}

func (b *airtestBufferedSpeaker) Stop() error {
	return b.cam.Stop()
}

func (b *airtestBufferedSpeaker) bufferStream(r io.Reader) (string, int64, error) {
	f, err := os.CreateTemp("", "airtest_*.raw")
	if err != nil {
		return "", 0, fmt.Errorf("creating temp file: %w", err)
	}
	path := f.Name()

	if b.output != "" {
		f.Close()
		os.Remove(path)
		path = b.output
		f, err = os.Create(path)
		if err != nil {
			return "", 0, fmt.Errorf("creating output file: %w", err)
		}
	}

	size, err := io.Copy(f, r)
	if closeErr := f.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		if b.output == "" {
			os.Remove(path)
		}
		return "", 0, fmt.Errorf("buffering audio: %w", err)
	}

	if b.output == "" {
		defer os.Remove(path)
	}

	return path, size, nil
}

// airtestEnv returns the value for a test-specific environment variable,
// checking CAMSPEAK_AIRTEST_* first and falling back to TEST_CAM_*.
func airtestEnv(suffix string) string {
	if v := os.Getenv("CAMSPEAK_AIRTEST_" + suffix); v != "" {
		return v
	}
	return os.Getenv("TEST_CAM_" + suffix)
}

// firstNonLoopbackIP returns the first non-loopback IPv4 address, or an empty
// string if none is found.
func firstNonLoopbackIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

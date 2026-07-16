package cameras

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// Go2rtcClient plays audio on a camera via go2rtc's stream-to-camera API.
// The go2rtc instance must have a stream configured with #backchannel=1
// for the target camera. camspeak sends audio by:
//  1. Starting a temporary HTTP server to serve the raw G.711ulaw file
//  2. POSTing to go2rtc: /api/streams?dst=<stream>&src=ffmpeg:http://<host>:<port>/file.raw#audio=pcmu#input=file
//  3. go2rtc fetches the file, transcodes via ffmpeg, and pushes RTP to the camera's backchannel
type Go2rtcClient struct {
	go2rtcURL string // e.g. "http://192.168.1.120:1984"
	stream    string // go2rtc stream name with backchannel (e.g. "garage_2way")
	ip        string // camera IP (for ping)
}

// NewGo2rtcClient creates a client that uses go2rtc's stream-to-camera API.
func NewGo2rtcClient(go2rtcURL, stream, ip string) *Go2rtcClient {
	return &Go2rtcClient{
		go2rtcURL: go2rtcURL,
		stream:    stream,
		ip:        ip,
	}
}

// SendRaw plays a raw G.711ulaw 8kHz file on the camera via go2rtc.
// It starts a temporary HTTP server to serve the file, then tells go2rtc
// to fetch and transcode it.
func (c *Go2rtcClient) SendRaw(rawFile string) error {
	// Start a temporary HTTP server to serve the raw file
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return fmt.Errorf("starting temp server: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port

	// Get our IP that go2rtc can reach (use the same host go2rtc is on,
	// or just use the first non-loopback IP)
	hostIP, err := getLocalIP()
	if err != nil {
		listener.Close()
		return fmt.Errorf("detecting local IP: %w", err)
	}

	fileName := filepath.Base(rawFile)
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.ServeFile(w, r, rawFile)
		}),
	}

	go func() {
		_ = srv.Serve(listener)
	}()

	defer func() {
		_ = srv.Close()
		listener.Close()
	}()

	// Build the go2rtc stream-to-camera API URL
	srcURL := fmt.Sprintf("ffmpeg:http://%s:%d/%s#audio=pcmu#input=file", hostIP, port, fileName)
	apiURL := fmt.Sprintf("%s/api/streams?dst=%s&src=%s",
		c.go2rtcURL,
		url.QueryEscape(c.stream),
		url.QueryEscape(srcURL),
	)

	slog.Info("go2rtc: streaming to camera", "stream", c.stream, "file", fileName, "src", srcURL)

	// Get file size for timeout calculation
	info, err := os.Stat(rawFile)
	if err != nil {
		return fmt.Errorf("stat raw file: %w", err)
	}

	// Timeout = playback duration + 10s grace (go2rtc needs time to connect + transcode)
	timeout := time.Duration(info.Size()/8000+10) * time.Second

	resp, err := (&http.Client{Timeout: timeout}).Post(apiURL, "text/plain", nil)
	if err != nil {
		return fmt.Errorf("go2rtc API call: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("go2rtc returned HTTP %d: %s", resp.StatusCode, body)
	}

	// Stop the stream (send empty src to clean up)
	stopURL := fmt.Sprintf("%s/api/streams?dst=%s&src=", c.go2rtcURL, url.QueryEscape(c.stream))
	stopResp, err := (&http.Client{Timeout: 5 * time.Second}).Post(stopURL, "text/plain", nil)
	if err == nil {
		stopResp.Body.Close()
	}

	return nil
}

// Ping checks if the camera is reachable via TCP on port 80.
func (c *Go2rtcClient) Ping() bool {
	return tcpPing(c.ip, 80, 5*time.Second)
}

// getLocalIP returns the first non-loopback IPv4 address of this host.
func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String(), nil
			}
		}
	}

	return "127.0.0.1", nil
}

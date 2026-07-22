package cameras

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	clog "github.com/charmbracelet/log"
)

// Go2rtcClient plays audio on a camera via go2rtc's stream-to-camera API.
// The go2rtc instance must have a stream configured with #backchannel=1
// for the target camera. camspeak sends audio by:
//  1. Starting a temporary HTTP server to serve the raw G.711ulaw file
//  2. POSTing to go2rtc: /api/streams?dst=<stream>&src=ffmpeg:http://<host>:<port>/file.raw#audio=pcmu#input=file
//  3. go2rtc fetches the file, transcodes via ffmpeg, and pushes RTP to the camera's backchannel
type Go2rtcClient struct {
	go2rtcURL   string // e.g. "http://192.168.1.120:1984"
	stream      string // go2rtc stream name with backchannel (e.g. "garage_2way")
	ip          string // camera IP (for ping)
	advertiseIP string // IP that go2rtc can reach camspeak on (for Docker, set to host IP)
	log         *clog.Logger

	// Active stream tracking for Stop()
	activeMu   sync.Mutex
	cancelFunc context.CancelFunc // cancels the active go2rtc API call
	stopped    bool               // set by Stop() to suppress errors
}

// NewGo2rtcClient creates a client that uses go2rtc's stream-to-camera API.
// advertiseIP is the IP that go2rtc should use to fetch the audio file from
// camspeak's temporary HTTP server. If empty, auto-detects the local IP.
// In Docker, set this to the host's LAN IP so go2rtc (on another host) can reach it.
func NewGo2rtcClient(go2rtcURL, stream, ip, advertiseIP string) *Go2rtcClient {
	return &Go2rtcClient{
		go2rtcURL:   go2rtcURL,
		stream:      stream,
		ip:          ip,
		advertiseIP: advertiseIP,
		log:         newLogger("go2rtc"),
	}
}

// SendRaw plays a raw G.711ulaw 8kHz file on the camera via go2rtc.
// It starts a temporary HTTP server to serve the file, then tells go2rtc
// to fetch and transcode it.
func (c *Go2rtcClient) SendRaw(rawFile string) error {
	// Reset stopped flag from any previous Stop() call
	c.activeMu.Lock()
	c.stopped = false
	c.activeMu.Unlock()

	// Start a temporary HTTP server to serve the raw file
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return fmt.Errorf("starting temp server: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port

	// Determine the IP that go2rtc can reach us on
	hostIP := c.advertiseIP
	if hostIP == "" {
		var err error
		hostIP, err = getLocalIP()
		if err != nil {
			listener.Close()
			return fmt.Errorf("detecting local IP: %w", err)
		}
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

	c.log.Info("streaming to camera", "stream", c.stream, "file", fileName, "src", srcURL)

	// Get file size for timeout calculation
	info, err := os.Stat(rawFile)
	if err != nil {
		return fmt.Errorf("stat raw file: %w", err)
	}

	// Timeout = playback duration + 10s grace (go2rtc needs time to connect + transcode)
	timeout := time.Duration(info.Size()/8000+10) * time.Second

	// Use cancellable context for Stop() support
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Track active cancel for Stop()
	c.activeMu.Lock()
	c.cancelFunc = cancel
	c.activeMu.Unlock()

	defer func() {
		c.activeMu.Lock()
		c.cancelFunc = nil
		c.activeMu.Unlock()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, nil)
	if err != nil {
		return fmt.Errorf("building go2rtc request: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		// Check if Stop() was called — intentional cancellation
		c.activeMu.Lock()
		wasStopped := c.stopped
		c.activeMu.Unlock()
		if wasStopped {
			c.log.Debug("send: stopped by user", "stream", c.stream)
			return nil
		}
		return fmt.Errorf("go2rtc API call: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512))
	if err != nil {
		c.log.Warn("go2rtc: reading response body failed", "err", err)
	}

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

// Stream is not yet implemented for go2rtc; it buffers r and calls SendRaw.
func (c *Go2rtcClient) Stream(r io.Reader) error {
	tmp, err := os.CreateTemp("", "camspeak-go2rtc-*.raw")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	return c.SendRaw(tmp.Name())
}

// Stop immediately stops audio playback by cancelling the active go2rtc API call
// and sending a stop command to go2rtc.
func (c *Go2rtcClient) Stop() error {
	c.activeMu.Lock()
	cancel := c.cancelFunc
	c.stopped = true // suppress errors in SendRaw
	c.activeMu.Unlock()

	c.log.Info("stop: stopping audio", "stream", c.stream)

	// Cancel the active HTTP request to go2rtc
	if cancel != nil {
		cancel()
	}

	// Also tell go2rtc to stop the stream
	stopURL := fmt.Sprintf("%s/api/streams?dst=%s&src=", c.go2rtcURL, url.QueryEscape(c.stream))
	stopResp, err := (&http.Client{Timeout: 5 * time.Second}).Post(stopURL, "text/plain", nil)
	if err == nil {
		stopResp.Body.Close()
	}

	c.activeMu.Lock()
	c.cancelFunc = nil
	c.activeMu.Unlock()

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

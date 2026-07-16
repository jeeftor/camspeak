package cameras

import (
	"fmt"
	"net/http"
	"time"
)

// ReolinkClient is a stub for Reolink doorbell audio.
// Reolink uses a different protocol than Hikvision ISAPI.
// Implementation options (in priority order):
//  1. go2rtc REST API (port 80) — if go2rtc exposes audio push
//  2. RTSP backchannel via ffmpeg subprocess
//  3. Reolink HTTP API (cmd=AudioAlarm)
type ReolinkClient struct {
	ip   string
	user string
	pass string
}

// NewReolinkClient creates a Reolink client.
func NewReolinkClient(ip, user, pass string) *ReolinkClient {
	return &ReolinkClient{ip: ip, user: user, pass: pass}
}

// SendRaw attempts to play audio on the Reolink doorbell speaker.
// Currently a stub — returns a clear error until implemented.
func (c *ReolinkClient) SendRaw(rawFile string) error {
	return fmt.Errorf("reolink audio not yet implemented for %s — "+
		"open an issue at github.com/jeeftor/camspeak", c.ip)
}

// Ping checks if the Reolink camera HTTP API is reachable on port 80.
func (c *ReolinkClient) Ping() bool {
	client := &http.Client{Timeout: 3 * time.Second}
	url := fmt.Sprintf("http://%s/cgi-bin/api.cgi?cmd=GetDevInfo", c.ip)

	resp, err := client.Get(url)
	if err != nil {
		return false
	}

	resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

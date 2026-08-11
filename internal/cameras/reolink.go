package cameras

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// ReolinkClient is a stub for Reolink camera audio.
//
// Reolink uses a proprietary Baichuan protocol (TCP port 9000) for two-way audio
// with IMA-ADPCM encoding (custom 516-byte block format). This is NOT implemented.
//
// The only workaround is routing through go2rtc with RTSP backchannel, which:
//   - Only works on Reolink Doorbells (not other models — see go2rtc#763)
//   - Only works on specific firmware (v3.0.0.2033_23041302 or earlier)
//   - Newer firmware (v3.0.0.3215) switched PCMA→PCMU and broke backchannel (go2rtc#987)
//   - Has ~3s latency (go2rtc#939) and backchannel can get stuck (go2rtc#1860)
//
// An unmerged go2rtc PR (#2263) adds native Baichuan protocol support with a
// reolink:// schema, but it is not in main go2rtc as of 2026-07.
//
// Reference implementations of the Baichuan protocol:
//   - neolink (Rust): https://github.com/thirtythreeforty/neolink
//   - nodelink-js (TypeScript): https://github.com/apocaliss92/nodelink-js
//   - reolink-aio (Python): https://github.com/starkillerOG/reolink_aio
type ReolinkClient struct {
	ip   string
	user string
	pass string
}

// NewReolinkClient creates a Reolink client.
func NewReolinkClient(ip, user, pass string) *ReolinkClient {
	return &ReolinkClient{ip: ip, user: user, pass: pass}
}

// SendRaw attempts to play audio on the Reolink camera speaker.
// Native Reolink two-way audio (Baichuan protocol) is not implemented.
// Audio must be routed through go2rtc with a backchannel-enabled stream.
func (c *ReolinkClient) SendRaw(rawFile string) (SendTiming, error) {
	return SendTiming{}, fmt.Errorf("reolink native audio not implemented for %s — "+
		"Reolink uses a proprietary Baichuan protocol (port 9000) that is not yet supported. "+
		"Workaround: configure a go2rtc stream with #backchannel=1 "+
		"(e.g. rtsp://USER:PASS@%s:554/h264Preview_01_sub#backchannel=1) "+
		"and set the stream name in the camera config. "+
		"Note: RTSP backchannel only works on Reolink Doorbells with specific firmware "+
		"(see go2rtc issues #763, #987, #939)",
		c.ip, c.ip)
}

// Stream is not yet implemented for Reolink.
func (c *ReolinkClient) Stream(_ io.Reader) error {
	return fmt.Errorf("reolink streaming not implemented for %s — "+
		"configure a go2rtc stream with #backchannel=1 and set the stream name. "+
		"Note: only works on Reolink Doorbells with specific firmware (see go2rtc#763)",
		c.ip)
}

// Stop is a no-op for Reolink (audio not yet implemented).
func (c *ReolinkClient) Stop() error {
	return nil
}

// Ping checks if the Reolink camera HTTP API is reachable on port 80.
// Falls back to a raw TCP connect if the HTTP request fails.
func (c *ReolinkClient) Ping() bool {
	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("http://%s/cgi-bin/api.cgi?cmd=GetDevInfo", c.ip)

	resp, err := client.Get(url)
	if err == nil {
		resp.Body.Close()
		// Any HTTP response means the camera is reachable
		return resp.StatusCode < 500
	}

	// Fallback: raw TCP connect
	return tcpPing(c.ip, 80, 3*time.Second)
}

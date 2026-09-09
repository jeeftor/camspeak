package cameras

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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

// reolinkLoginResponse is the JSON response from the Reolink login API.
type reolinkLoginResponse []struct {
	Cmd   string `json:"cmd"`
	Code  int    `json:"code"`
	Value struct {
		Token struct {
			Name      string `json:"name"`
			LeaseTime int    `json:"leaseTime"`
		} `json:"Token"`
	} `json:"value"`
}

// reolinkSnapResponse is the JSON error response (if any) from the Snap API.
type reolinkSnapResponse []struct {
	Cmd   string `json:"cmd"`
	Code  int    `json:"code"`
	Error struct {
		Detail  string `json:"detail"`
		RspCode int    `json:"rspCode"`
	} `json:"error"`
}

// Snapshot captures a single JPEG frame from a Reolink camera.
// streamType is "main" or "sub" (channel 0 or 1 in Reolink's API).
// Uses the Reolink HTTP API: login to get a token, then /cgi-bin/api.cgi?cmd=Snap.
func (c *ReolinkClient) Snapshot(streamType string) ([]byte, error) {
	channel := 0
	if streamType == "sub" {
		channel = 1
	}

	client := &http.Client{Timeout: 10 * time.Second}
	baseURL := fmt.Sprintf("http://%s/cgi-bin/api.cgi", c.ip)

	// Step 1: Login to get a token.
	loginBody := fmt.Sprintf(
		`[{"cmd":"Login","param":{"User":{"userName":"%s","password":"%s"}}}]`,
		c.user, c.pass,
	)
	loginURL := baseURL + "?cmd=Login&source=null"
	resp, err := client.Post(loginURL, "application/json", stringReader(loginBody))
	if err != nil {
		return nil, fmt.Errorf("reolink login: %w", err)
	}
	loginData, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("reolink login returned HTTP %d", resp.StatusCode)
	}

	var loginResp reolinkLoginResponse
	if err := json.Unmarshal(loginData, &loginResp); err != nil {
		return nil, fmt.Errorf("reolink login: invalid JSON response")
	}
	if len(loginResp) == 0 || loginResp[0].Code != 0 {
		return nil, fmt.Errorf("reolink login failed (code=%d)", loginResp[0].Code)
	}
	token := loginResp[0].Value.Token.Name
	if token == "" {
		return nil, fmt.Errorf("reolink login: empty token")
	}

	// Step 2: Capture snapshot using the token.
	snapURL := fmt.Sprintf(
		"%s?cmd=Snap&channel=%d&rs=%s&token=%s",
		baseURL, channel, fmt.Sprintf("%d", time.Now().UnixNano()), url.QueryEscape(token),
	)
	resp, err = client.Get(snapURL)
	if err != nil {
		return nil, fmt.Errorf("reolink snap: %w", err)
	}
	defer resp.Body.Close()

	// Reolink returns either a JPEG (image/jpeg) or a JSON error array.
	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "image/") {
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("reolink snap: reading body: %w", err)
		}
		if len(data) < 100 {
			return nil, fmt.Errorf("reolink snap: too little data (%d bytes)", len(data))
		}
		return data, nil
	}

	// JSON error response
	errData, _ := io.ReadAll(resp.Body)
	var snapErr reolinkSnapResponse
	if json.Unmarshal(errData, &snapErr) == nil && len(snapErr) > 0 && snapErr[0].Code != 0 {
		return nil, fmt.Errorf("reolink snap failed: %s (code=%d)",
			snapErr[0].Error.Detail, snapErr[0].Error.RspCode)
	}
	return nil, fmt.Errorf("reolink snap: unexpected content-type %s", ct)
}

// stringReader wraps a string as an io.Reader.
func stringReader(s string) io.Reader {
	return &stringReaderImpl{s: s}
}

type stringReaderImpl struct {
	s   string
	pos int
}

func (r *stringReaderImpl) Read(p []byte) (int, error) {
	if r.pos >= len(r.s) {
		return 0, io.EOF
	}
	n := copy(p, r.s[r.pos:])
	r.pos += n
	return n, nil
}

// SendRaw attempts to play audio on the Reolink camera speaker.
// Native Reolink two-way audio (Baichuan protocol) is not implemented.
// Audio must be routed through go2rtc with a backchannel-enabled stream.
func (c *ReolinkClient) SendRaw(rawFile string, gc *GainController) (SendTiming, error) {
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

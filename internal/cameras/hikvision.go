// Package cameras provides audio streaming clients for camera types.
package cameras

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	clog "github.com/charmbracelet/log"
	"github.com/icholy/digest"
)

// HikvisionClient sends audio to a Hikvision camera via ISAPI Two-Way Audio.
type HikvisionClient struct {
	ip      string
	user    string
	pass    string
	channel int
	client  *http.Client
	log     *clog.Logger
}

// NewHikvisionClient creates a client with digest auth transport.
func NewHikvisionClient(ip, user, pass string, channel int) *HikvisionClient {
	transport := &digest.Transport{
		Username: user,
		Password: pass,
	}

	return &HikvisionClient{
		ip:      ip,
		user:    user,
		pass:    pass,
		channel: channel,
		client:  &http.Client{Transport: transport},
		log:     newLogger("hikvision"),
	}
}

func (c *HikvisionClient) baseURL() string {
	return fmt.Sprintf("http://%s/ISAPI/System/TwoWayAudio/channels/%d", c.ip, c.channel)
}

// openResponse is the XML response from /open.
type openResponse struct {
	SessionID string `xml:"sessionId"`
}

// openChannel opens the two-way audio session and returns the sessionId.
func (c *HikvisionClient) openChannel() (string, error) {
	// Clear any stale session first (ignore error).
	req, _ := http.NewRequest(http.MethodPut, c.baseURL()+"/close", nil)
	if req != nil {
		if resp, err := c.client.Do(req); err == nil {
			resp.Body.Close()
		}
	}

	req, err := http.NewRequest(http.MethodPut, c.baseURL()+"/open", nil)
	if err != nil {
		return "", fmt.Errorf("building open request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("opening channel: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading open response: %w", err)
	}

	var result openResponse
	if err := xml.Unmarshal(body, &result); err != nil || result.SessionID == "" {
		return "", fmt.Errorf("no sessionId in response (HTTP %d): %s", resp.StatusCode, body)
	}

	return result.SessionID, nil
}

// closeChannel closes the two-way audio session.
func (c *HikvisionClient) closeChannel(sessionID string) {
	url := fmt.Sprintf("%s/close?sessionId=%s", c.baseURL(), sessionID)

	req, err := http.NewRequest(http.MethodPut, url, nil)
	if err != nil {
		return
	}

	if resp, err := c.client.Do(req); err == nil {
		resp.Body.Close()
	}
}

// SendRaw streams a raw G.711ulaw file to the camera speaker.
// The file must already be G.711ulaw 8kHz raw (8000 bytes/sec).
// Rate is throttled to 8000 bytes/sec to match real-time playback.
//
// The file is read into memory so that GetBody can produce a fresh
// throttled reader for each attempt. The digest transport sends the
// request twice (first gets 401 challenge, second sends with auth);
// if we don't set GetBody, the transport caches the body as a plain
// bytes.Reader and the retry sends everything unthrottled — the
// camera drops or distorts the audio.
func (c *HikvisionClient) SendRaw(rawFile string) error {
	data, err := os.ReadFile(rawFile)
	if err != nil {
		return fmt.Errorf("reading audio file: %w", err)
	}

	size := int64(len(data))
	c.log.Info("send: opening channel", "ip", c.ip, "channel", c.channel, "bytes", size, "duration_s", size/8000)

	openStart := time.Now()
	sessionID, err := c.openChannel()
	if err != nil {
		c.log.Error("send: open channel failed", "ip", c.ip, "err", err)
		return fmt.Errorf("open channel: %w", err)
	}
	c.log.Debug("send: channel opened", "session", sessionID, "elapsed", time.Since(openStart))
	defer c.closeChannel(sessionID)

	// Timeout = playback duration + 2s grace (digest retry needs headroom)
	timeout := time.Duration(size/8000+2) * time.Second

	url := fmt.Sprintf("%s/audioData?sessionId=%s", c.baseURL(), sessionID)

	// getBody returns a fresh throttled reader each time the digest
	// transport needs to resend the request (401 → authenticated retry).
	getBody := func() (io.ReadCloser, error) {
		return io.NopCloser(newThrottledReader(bytes.NewReader(data), 8000)), nil
	}

	bodyReader, err := getBody()
	if err != nil {
		return fmt.Errorf("building audio body: %w", err)
	}
	req, err := http.NewRequest(http.MethodPut, url, bodyReader)
	if err != nil {
		return fmt.Errorf("building audio request: %w", err)
	}
	req.GetBody = getBody

	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = size

	httpClient := &http.Client{
		Transport: &digest.Transport{Username: c.user, Password: c.pass},
		Timeout:   timeout,
	}

	c.log.Info("send: streaming audio", "ip", c.ip, "session", sessionID, "bytes", size)
	sendStart := time.Now()

	resp, err := httpClient.Do(req)
	if err != nil {
		// Timeout after upload = we sent everything, camera is still playing. That's fine.
		if isTimeoutError(err) {
			c.log.Info("send: complete (timeout after upload)", "ip", c.ip, "bytes", size, "elapsed", time.Since(sendStart))
			return nil
		}
		c.log.Error("send: upload failed", "ip", c.ip, "err", err)
		return fmt.Errorf("sending audio: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		c.log.Error("send: camera rejected audio", "ip", c.ip, "status", resp.StatusCode, "body", string(body))
		return fmt.Errorf("camera returned HTTP %d: %s", resp.StatusCode, body)
	}

	c.log.Info("send: complete", "ip", c.ip, "bytes", size, "elapsed", time.Since(sendStart))
	return nil
}

// Ping checks if the camera ISAPI is reachable.
// Uses digest auth to hit /ISAPI/System/deviceInfo with a 5s timeout.
// Falls back to a raw TCP connect check if the HTTP request fails
// (e.g. wrong credentials but camera is still on the network).
func (c *HikvisionClient) Ping() bool {
	// Try authenticated ISAPI request first
	req, err := http.NewRequest(http.MethodGet,
		fmt.Sprintf("http://%s/ISAPI/System/deviceInfo", c.ip), nil)
	if err != nil {
		return false
	}

	client := &http.Client{
		Transport: &digest.Transport{Username: c.user, Password: c.pass},
		Timeout:   5 * time.Second,
	}

	resp, err := client.Do(req)
	if err == nil {
		resp.Body.Close()
		// 200 = fully online with valid creds
		// 401 = camera is reachable but creds are wrong — still "online"
		return resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized
	}

	// Fallback: raw TCP connect to port 80
	return tcpPing(c.ip, 80, 3*time.Second)
}

// throttledReader wraps an io.Reader and rate-limits reads to bytesPerSec bytes/sec.
type throttledReader struct {
	r           io.Reader
	bytesPerSec int64
}

func newThrottledReader(r io.Reader, bytesPerSec int64) io.Reader {
	return &throttledReader{r: r, bytesPerSec: bytesPerSec}
}

// Read implements io.Reader with rate limiting (800 bytes per 100ms chunk).
func (t *throttledReader) Read(p []byte) (int, error) {
	chunkSize := t.bytesPerSec / 10 // 100ms chunks
	if int64(len(p)) > chunkSize {
		p = p[:chunkSize]
	}

	n, err := t.r.Read(p)
	if n > 0 {
		time.Sleep(100 * time.Millisecond)
	}

	return n, err
}

func isTimeoutError(err error) bool {
	if err == nil {
		return false
	}

	type timeoutErr interface{ Timeout() bool }

	if te, ok := err.(timeoutErr); ok {
		return te.Timeout()
	}

	return false
}

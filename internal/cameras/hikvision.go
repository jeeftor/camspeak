// Package cameras provides audio streaming clients for camera types.
package cameras

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
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
	mu      sync.Mutex // serializes audio sends — camera supports one session at a time
	log     *clog.Logger

	// Active stream tracking for Stop()
	activeMu      sync.Mutex
	activeConn    net.Conn // active TCP connection streaming audio
	activeSession string   // active ISAPI two-way audio session ID
	stopped       bool     // set by Stop() to suppress write errors in SendRaw
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
		if resp.StatusCode == http.StatusUnauthorized {
			return "", fmt.Errorf(
				"camera authentication failed (HTTP 401) — check username/password in Config → Cameras",
			)
		}
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
// This uses a raw TCP connection instead of net/http because the
// Hikvision camera returns HTTP 200 after receiving only the first
// chunk of body data. Go's HTTP client stops reading the request body
// when it receives the response, so the remaining audio is never sent.
// By using a raw TCP connection, we can write all the data at 8000
// bytes/sec before reading the response — matching curl's behavior
// with --limit-rate.
func (c *HikvisionClient) SendRaw(rawFile string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Reset stopped flag from any previous Stop() call
	c.activeMu.Lock()
	c.stopped = false
	c.activeMu.Unlock()

	data, err := os.ReadFile(rawFile)
	if err != nil {
		return fmt.Errorf("reading audio file: %w", err)
	}

	size := int64(len(data))
	c.log.Info(
		"send: opening channel",
		"ip",
		c.ip,
		"channel",
		c.channel,
		"bytes",
		size,
		"duration_s",
		size/8000,
	)

	openStart := time.Now()
	sessionID, err := c.openChannel()
	if err != nil {
		c.log.Error("send: open channel failed", "ip", c.ip, "err", err)
		return fmt.Errorf("open channel: %w", err)
	}
	c.log.Debug("send: channel opened", "session", sessionID, "elapsed", time.Since(openStart))

	// Track active session for Stop()
	c.activeMu.Lock()
	c.activeSession = sessionID
	c.activeMu.Unlock()

	defer func() {
		c.closeChannel(sessionID)
		c.activeMu.Lock()
		c.activeConn = nil
		c.activeSession = ""
		c.activeMu.Unlock()
	}()

	c.log.Info("send: streaming audio", "ip", c.ip, "session", sessionID, "bytes", size)
	sendStart := time.Now()

	if err := c.sendAudioRaw(sessionID, data); err != nil {
		c.log.Error("send: upload failed", "ip", c.ip, "err", err)
		return fmt.Errorf("sending audio: %w", err)
	}

	c.log.Info("send: complete", "ip", c.ip, "bytes", size, "elapsed", time.Since(sendStart))
	return nil
}

// Stream opens a single long-lived ISAPI two-way audio session and copies r
// to the camera speaker at 8000 bytes/sec until r returns EOF.
// This is the preferred method for AirPlay; it avoids per-chunk open/close overhead.
func (c *HikvisionClient) Stream(r io.Reader) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.activeMu.Lock()
	c.stopped = false
	c.activeMu.Unlock()

	sessionID, err := c.openChannel()
	if err != nil {
		return fmt.Errorf("open channel: %w", err)
	}
	defer c.closeChannel(sessionID)

	path := fmt.Sprintf(
		"/ISAPI/System/TwoWayAudio/channels/%d/audioData?sessionId=%s",
		c.channel, sessionID,
	)

	authHeader, err := c.getDigestAuth(path)
	if err != nil {
		return fmt.Errorf("digest auth: %w", err)
	}

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(c.ip, "80"), 5*time.Second)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	c.activeMu.Lock()
	c.activeConn = conn
	c.activeSession = sessionID
	c.activeMu.Unlock()
	defer func() {
		c.activeMu.Lock()
		c.activeConn = nil
		c.activeSession = ""
		c.activeMu.Unlock()
	}()

	// Use a large Content-Length (1 hour); we close the TCP connection early when done.
	const maxBytes = 3600 * 8000
	headers := fmt.Sprintf("PUT %s HTTP/1.1\r\n", path)
	headers += fmt.Sprintf("Host: %s\r\n", c.ip)
	headers += "Content-Type: application/octet-stream\r\n"
	headers += fmt.Sprintf("Content-Length: %d\r\n", maxBytes)
	if authHeader != "" {
		headers += fmt.Sprintf("Authorization: %s\r\n", authHeader)
	}
	headers += "Connection: close\r\n\r\n"

	if _, err := conn.Write([]byte(headers)); err != nil {
		return fmt.Errorf("write headers: %w", err)
	}

	c.log.Info("stream: session open", "ip", c.ip, "session", sessionID)
	err = copyAt8kBps(conn, r, &c.stopped, &c.activeMu)
	c.log.Info("stream: session closed", "ip", c.ip, "session", sessionID)
	return err
}

// getDigestAuth performs the 401 challenge/response handshake for the given path
// and returns the Authorization header value.
func (c *HikvisionClient) getDigestAuth(path string) (string, error) {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(c.ip, "80"), 5*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	probe := fmt.Sprintf("PUT %s HTTP/1.1\r\nHost: %s\r\nContent-Length: 0\r\n\r\n", path, c.ip)
	if _, err := conn.Write([]byte(probe)); err != nil {
		return "", err
	}

	r := bufio.NewReader(conn)
	if _, err := r.ReadString('\n'); err != nil {
		return "", err
	}
	var wwwAuth string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "www-authenticate:") {
			wwwAuth = strings.TrimSpace(line[len("www-authenticate:"):])
		}
	}
	if wwwAuth == "" {
		return "", nil
	}
	chal, err := digest.FindChallenge(http.Header{"Www-Authenticate": []string{wwwAuth}})
	if err != nil {
		return "", err
	}
	cred, err := digest.Digest(chal, digest.Options{
		Method:   http.MethodPut,
		URI:      path,
		Username: c.user,
		Password: c.pass,
	})
	if err != nil {
		return "", err
	}
	return cred.String(), nil
}

// copyAt8kBps copies r → w paced at 8000 bytes/sec (G.711 mulaw real-time rate).
func copyAt8kBps(w io.Writer, r io.Reader, stopped *bool, mu *sync.Mutex) error {
	const chunkSize = 800 // 100ms at 8000 bytes/sec
	const interval = 100 * time.Millisecond

	buf := make([]byte, chunkSize)
	next := time.Now()

	for {
		n, err := io.ReadFull(r, buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				mu.Lock()
				s := *stopped
				mu.Unlock()
				if s {
					return nil
				}
				return werr
			}
			next = next.Add(interval)
			if sleep := time.Until(next); sleep > 0 {
				time.Sleep(sleep)
			}
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// Stop immediately stops audio playback by closing the active TCP connection
// and the ISAPI two-way audio channel.
func (c *HikvisionClient) Stop() error {
	c.activeMu.Lock()
	conn := c.activeConn
	sessionID := c.activeSession
	c.stopped = true // suppress write errors in the streaming loop
	c.activeMu.Unlock()

	if conn == nil && sessionID == "" {
		return nil // nothing playing
	}

	c.log.Info("stop: stopping audio", "ip", c.ip, "session", sessionID)

	// Close the TCP connection — this interrupts the streaming loop
	if conn != nil {
		_ = conn.Close()
	}

	// Close the ISAPI two-way audio channel
	if sessionID != "" {
		c.closeChannel(sessionID)
	}

	c.activeMu.Lock()
	c.activeConn = nil
	c.activeSession = ""
	c.activeMu.Unlock()

	return nil
}

// sendAudioRaw opens a raw TCP connection and sends the audio data
// with digest auth, throttled to 8000 bytes/sec.
func (c *HikvisionClient) sendAudioRaw(sessionID string, data []byte) error {
	path := fmt.Sprintf(
		"/ISAPI/System/TwoWayAudio/channels/%d/audioData?sessionId=%s",
		c.channel,
		sessionID,
	)
	host := c.ip
	port := "80"

	// Step 1: Open TCP connection for the digest challenge
	conn1, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 5*time.Second)
	if err != nil {
		return fmt.Errorf("dialing camera: %w", err)
	}

	// Send a PUT with Content-Length: 0 to get the 401 challenge
	headReq := fmt.Sprintf("PUT %s HTTP/1.1\r\nHost: %s\r\nContent-Length: 0\r\n\r\n", path, host)
	if _, err := conn1.Write([]byte(headReq)); err != nil {
		conn1.Close()
		return fmt.Errorf("sending challenge request: %w", err)
	}

	// Read the 401 response and extract WWW-Authenticate header
	respReader := bufio.NewReader(conn1)
	_, err = respReader.ReadString('\n')
	if err != nil {
		conn1.Close()
		return fmt.Errorf("reading status line: %w", err)
	}

	var wwwAuth string
	for {
		line, err := respReader.ReadString('\n')
		if err != nil {
			break
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "www-authenticate:") {
			wwwAuth = strings.TrimSpace(line[len("www-authenticate:"):])
		}
	}
	conn1.Close()

	var authHeader string
	if wwwAuth != "" {
		// Parse the digest challenge and compute credentials
		chal, err := digest.FindChallenge(http.Header{"Www-Authenticate": []string{wwwAuth}})
		if err != nil {
			return fmt.Errorf("parsing digest challenge: %w", err)
		}

		cred, err := digest.Digest(chal, digest.Options{
			Method:   http.MethodPut,
			URI:      path,
			Username: c.user,
			Password: c.pass,
		})
		if err != nil {
			return fmt.Errorf("computing digest credentials: %w", err)
		}
		authHeader = cred.String()
	}

	// Step 2: Open a NEW TCP connection for the authenticated request
	conn2, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 5*time.Second)
	if err != nil {
		return fmt.Errorf("dialing camera for audio: %w", err)
	}
	defer conn2.Close()

	// Track active connection for Stop()
	c.activeMu.Lock()
	c.activeConn = conn2
	c.activeMu.Unlock()

	// Set deadline = playback duration + 5s grace
	deadline := time.Now().Add(time.Duration(int64(len(data))/8000+5) * time.Second)
	_ = conn2.SetDeadline(deadline)

	return c.sendAudioWithAuth(conn2, path, host, authHeader, data)
}

// sendAudioWithAuth sends the PUT request with the audio body, throttled to 8000 bytes/sec.
func (c *HikvisionClient) sendAudioWithAuth(
	conn net.Conn,
	path, host, authHeader string,
	data []byte,
) error {
	size := int64(len(data))

	// Build request headers
	var headers string
	headers += fmt.Sprintf("PUT %s HTTP/1.1\r\n", path)
	headers += fmt.Sprintf("Host: %s\r\n", host)
	headers += "Content-Type: application/octet-stream\r\n"
	headers += fmt.Sprintf("Content-Length: %d\r\n", size)
	if authHeader != "" {
		headers += fmt.Sprintf("Authorization: %s\r\n", authHeader)
	}
	headers += "Connection: close\r\n"
	headers += "\r\n"

	if _, err := conn.Write([]byte(headers)); err != nil {
		return fmt.Errorf("writing request headers: %w", err)
	}

	// Write body at 8000 bytes/sec (800 bytes per 100ms chunk)
	chunkSize := 800
	totalWritten := 0
	for totalWritten < len(data) {
		end := totalWritten + chunkSize
		if end > len(data) {
			end = len(data)
		}
		n, err := conn.Write(data[totalWritten:end])
		if err != nil {
			// Check if Stop() was called — if so, this is an intentional
			// cancellation, not a real error.
			c.activeMu.Lock()
			wasStopped := c.stopped
			c.activeMu.Unlock()
			if wasStopped {
				c.log.Debug("send: stopped by user", "written", totalWritten+n, "total", size)
				return nil
			}
			// If we've written most of the data, the camera may have closed
			// after receiving enough — treat as success if we wrote >50%.
			if int64(totalWritten) > size/2 {
				c.log.Debug(
					"send: write interrupted (partial send)",
					"written",
					totalWritten+n,
					"total",
					size,
					"err",
					err,
				)
				return nil
			}
			return fmt.Errorf("writing audio data: %w", err)
		}
		totalWritten += n
		if totalWritten < len(data) {
			time.Sleep(100 * time.Millisecond)
		}
	}

	c.log.Debug("send: body written", "bytes", totalWritten, "expected", size)

	// Read response (camera may have already sent it or will send after playback)
	respReader := bufio.NewReader(conn)
	statusLine, err := respReader.ReadString('\n')
	if err != nil {
		// Timeout or connection closed after full send — that's fine,
		// the camera got all the data.
		c.log.Debug("send: no response (connection closed after send)", "err", err)
		return nil
	}

	// Check status code
	if !strings.Contains(statusLine, "200") && !strings.Contains(statusLine, "204") {
		// Read a bit of body for error context
		body, _ := respReader.ReadString('\n')
		return fmt.Errorf(
			"camera returned %s: %s",
			strings.TrimSpace(statusLine),
			strings.TrimSpace(body),
		)
	}

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

package airplay

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// mockSpeaker is a test Speaker that records SendRaw calls.
type mockSpeaker struct {
	rawFiles []string
	stopErr  error
}

func (m *mockSpeaker) SendRaw(rawFile string) error {
	m.rawFiles = append(m.rawFiles, rawFile)
	return nil
}

func (m *mockSpeaker) Stop() error {
	return m.stopErr
}

// TestRTSPFullSession tests a complete RAOP session over a real TCP connection:
// OPTIONS → ANNOUNCE → SETUP → RECORD → TEARDOWN
func TestRTSPFullSession(t *testing.T) {
	speaker := &mockSpeaker{}

	// Create server on a random port
	server, err := NewServer("test-camera", 0, "", speaker)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	// Start listening (but skip mDNS registration for test)
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	server.listener = ln
	port := ln.Addr().(*net.TCPAddr).Port
	go server.acceptLoop()
	defer server.Stop()

	// Connect as a fake iOS client
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("net.Dial failed: %v", err)
	}
	defer conn.Close()

	// Use a persistent reader for the entire session
	reader := bufio.NewReader(conn)

	// --- Step 1: OPTIONS ---
	resp := sendRTSPWithReader(t, conn, reader, "OPTIONS * RTSP/1.0\r\nCSeq: 1\r\n\r\n")
	if !strings.Contains(resp, "200 OK") {
		t.Fatalf("OPTIONS failed: %s", resp)
	}
	if !strings.Contains(resp, "ANNOUNCE") {
		t.Errorf("OPTIONS response missing ANNOUNCE: %s", resp)
	}

	// --- Step 2: ANNOUNCE (with RSA challenge + AES key) ---
	privKey := server.rsaKey
	aesKey := make([]byte, 16)
	rand.Read(aesKey)
	encryptedAesKey, _ := rsa.EncryptOAEP(sha1.New(), rand.Reader, &privKey.PublicKey, aesKey, nil)
	rsaAesKeyB64 := strings.TrimRight(base64.StdEncoding.EncodeToString(encryptedAesKey), "=")

	aesIV := make([]byte, 16)
	rand.Read(aesIV)
	aesIVB64 := strings.TrimRight(base64.StdEncoding.EncodeToString(aesIV), "=")

	challenge := make([]byte, 16)
	rand.Read(challenge)
	challengeB64 := strings.TrimRight(base64.StdEncoding.EncodeToString(challenge), "=")

	sdp := "v=0\r\n" +
		"o=iTunes 0 0 IN IP4 0.0.0.0\r\n" +
		"s=iTunes\r\n" +
		"c=IN IP4 0.0.0.0\r\n" +
		"t=0 0\r\n" +
		"m=audio 0 RTP/AVP 96\r\n" +
		"a=rtpmap:96 AppleLossless\r\n" +
		"a=fmtp:96 352 0 16 40 10 14 2 255 0 0 44100\r\n" +
		"a=rsaaeskey:" + rsaAesKeyB64 + "\r\n" +
		"a=aesiv:" + aesIVB64 + "\r\n"

	announceReq := fmt.Sprintf(
		"ANNOUNCE rtsp://test/123 RTSP/1.0\r\n"+
			"CSeq: 2\r\n"+
			"Content-Type: application/sdp\r\n"+
			"Content-Length: %d\r\n"+
			"Apple-Challenge: %s\r\n"+
			"\r\n"+
			"%s",
		len(sdp), challengeB64, sdp,
	)

	resp = sendRTSPWithReader(t, conn, reader, announceReq)
	if !strings.Contains(resp, "200 OK") {
		t.Fatalf("ANNOUNCE failed: %s", resp)
	}
	if !strings.Contains(resp, "Apple-Response:") {
		t.Errorf("ANNOUNCE response missing Apple-Response: %s", resp)
	}
	if !strings.Contains(resp, "Audio-Jack-Status:") {
		t.Errorf("ANNOUNCE response missing Audio-Jack-Status: %s", resp)
	}

	// --- Step 3: SETUP ---
	setupReq := "SETUP rtsp://test/123 RTSP/1.0\r\n" +
		"CSeq: 3\r\n" +
		"Transport: RTP/AVP/UDP;unicast;interleaved=0-1;mode=record;control_port=6001;timing_port=6002\r\n" +
		"\r\n"

	resp = sendRTSPWithReader(t, conn, reader, setupReq)
	if !strings.Contains(resp, "200 OK") {
		t.Fatalf("SETUP failed: %s", resp)
	}
	if !strings.Contains(resp, "Session:") {
		t.Errorf("SETUP response missing Session header: %s", resp)
	}
	if !strings.Contains(resp, "Transport:") {
		t.Errorf("SETUP response missing Transport header: %s", resp)
	}
	if !strings.Contains(resp, "server_port=") {
		t.Errorf("SETUP response missing server_port in Transport: %s", resp)
	}
	if !strings.Contains(resp, "control_port=") {
		t.Errorf("SETUP response missing control_port in Transport: %s", resp)
	}
	if !strings.Contains(resp, "timing_port=") {
		t.Errorf("SETUP response missing timing_port in Transport: %s", resp)
	}

	// --- Step 4: RECORD ---
	// RECORD starts the ffmpeg audio pipeline. If ffmpeg is not installed
	// (e.g. in CI), the server returns 500 — that's OK, the RTSP protocol
	// flow is still valid. We just skip the Audio-Latency check in that case.
	recordReq := "RECORD rtsp://test/123 RTSP/1.0\r\n" +
		"CSeq: 4\r\n" +
		"Session: 1\r\n" +
		"Range: npt=0-\r\n" +
		"RTP-Info: seq=0;rtptime=0\r\n" +
		"\r\n"

	resp = sendRTSPWithReader(t, conn, reader, recordReq)
	if strings.Contains(resp, "200 OK") {
		if !strings.Contains(resp, "Audio-Latency:") {
			t.Errorf("RECORD response missing Audio-Latency: %s", resp)
		}
	} else if !strings.Contains(resp, "500") {
		t.Fatalf("RECORD should return 200 or 500, got: %s", resp)
	}

	// --- Step 5: TEARDOWN ---
	teardownReq := "TEARDOWN rtsp://test/123 RTSP/1.0\r\n" +
		"CSeq: 5\r\n" +
		"Session: 1\r\n" +
		"\r\n"

	resp = sendRTSPWithReader(t, conn, reader, teardownReq)
	if !strings.Contains(resp, "200 OK") {
		t.Fatalf("TEARDOWN failed: %s", resp)
	}

	// Verify session was cleaned up
	server.sessionMu.Lock()
	if server.session != nil {
		t.Error("session was not cleaned up after TEARDOWN")
	}
	server.sessionMu.Unlock()
}

// TestRTSPMultipleSessions tests that a new ANNOUNCE replaces the old session.
func TestRTSPMultipleSessions(t *testing.T) {
	speaker := &mockSpeaker{}
	server, err := NewServer("test-camera", 0, "", speaker)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	server.listener = ln
	port := ln.Addr().(*net.TCPAddr).Port
	go server.acceptLoop()
	defer server.Stop()

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("net.Dial failed: %v", err)
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)

	// First ANNOUNCE
	aesKey1 := make([]byte, 16)
	rand.Read(aesKey1)
	enc1, _ := rsa.EncryptOAEP(sha1.New(), rand.Reader, &server.rsaKey.PublicKey, aesKey1, nil)
	b64_1 := strings.TrimRight(base64.StdEncoding.EncodeToString(enc1), "=")

	aesIV := make([]byte, 16)
	rand.Read(aesIV)
	ivB64 := strings.TrimRight(base64.StdEncoding.EncodeToString(aesIV), "=")

	sdp1 := "v=0\r\nm=audio 0 RTP/AVP 96\r\na=rsaaeskey:" + b64_1 + "\r\na=aesiv:" + ivB64 + "\r\n"
	req1 := fmt.Sprintf(
		"ANNOUNCE rtsp://test/1 RTSP/1.0\r\nCSeq: 1\r\nContent-Length: %d\r\n\r\n%s",
		len(sdp1),
		sdp1,
	)
	resp := sendRTSPWithReader(t, conn, reader, req1)
	if !strings.Contains(resp, "200 OK") {
		t.Fatalf("First ANNOUNCE failed: %s", resp)
	}

	// Second ANNOUNCE (should replace first session)
	aesKey2 := make([]byte, 16)
	rand.Read(aesKey2)
	enc2, _ := rsa.EncryptOAEP(sha1.New(), rand.Reader, &server.rsaKey.PublicKey, aesKey2, nil)
	b64_2 := strings.TrimRight(base64.StdEncoding.EncodeToString(enc2), "=")

	sdp2 := "v=0\r\nm=audio 0 RTP/AVP 96\r\na=rsaaeskey:" + b64_2 + "\r\na=aesiv:" + ivB64 + "\r\n"
	req2 := fmt.Sprintf(
		"ANNOUNCE rtsp://test/2 RTSP/1.0\r\nCSeq: 2\r\nContent-Length: %d\r\n\r\n%s",
		len(sdp2),
		sdp2,
	)
	resp = sendRTSPWithReader(t, conn, reader, req2)
	if !strings.Contains(resp, "200 OK") {
		t.Fatalf("Second ANNOUNCE failed: %s", resp)
	}

	// Verify the session has the second AES key
	server.sessionMu.Lock()
	sess := server.session
	server.sessionMu.Unlock()
	if sess == nil {
		t.Fatal("session is nil after second ANNOUNCE")
	}
	for i := range aesKey2 {
		if sess.aesKey[i] != aesKey2[i] {
			t.Errorf("session AES key doesn't match second key at byte %d", i)
		}
	}
}

// TestRTSPUnknownMethod tests that unknown methods return 405.
func TestRTSPUnknownMethod(t *testing.T) {
	speaker := &mockSpeaker{}
	server, err := NewServer("test-camera", 0, "", speaker)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	server.listener = ln
	port := ln.Addr().(*net.TCPAddr).Port
	go server.acceptLoop()
	defer server.Stop()

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("net.Dial failed: %v", err)
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)

	resp := sendRTSPWithReader(t, conn, reader, "DESCRIBE rtsp://test RTSP/1.0\r\nCSeq: 1\r\n\r\n")
	if !strings.Contains(resp, "405") {
		t.Errorf("Unknown method should return 405, got: %s", resp)
	}
}

// --- Helper ---

// sendRTSPWithReader sends a raw RTSP request and reads the response using a
// persistent bufio.Reader (to avoid losing buffered bytes between calls).
func sendRTSPWithReader(t *testing.T, conn net.Conn, reader *bufio.Reader, req string) string {
	t.Helper()
	_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	// Read status line
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read status line failed: %v", err)
	}

	var resp strings.Builder
	resp.WriteString(line)

	// Read headers
	for {
		line, err = reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("read header failed: %v", err)
		}
		resp.WriteString(line)
		if strings.TrimSpace(line) == "" {
			break
		}
	}

	// Check for Content-Length and read body if present
	for _, h := range strings.Split(resp.String(), "\r\n") {
		if strings.HasPrefix(strings.ToLower(h), "content-length:") {
			var n int
			_, _ = fmt.Sscanf(strings.TrimPrefix(h, "Content-Length: "), "%d", &n)
			if n > 0 {
				buf := make([]byte, n)
				_, _ = io.ReadFull(reader, buf)
				resp.Write(buf)
			}
			break
		}
	}

	return resp.String()
}

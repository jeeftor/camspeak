package airplay

import (
	"bufio"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"encoding/base64"
	"os"
	"strings"
	"testing"

	clog "github.com/charmbracelet/log"
)

// --- RSA Key Tests ---

func TestLoadRSAPrivateKey(t *testing.T) {
	key, err := loadRSAPrivateKey()
	if err != nil {
		t.Fatalf("loadRSAPrivateKey failed: %v", err)
	}
	if key == nil {
		t.Fatal("key is nil")
	}
	// The AirPort Express key is 2048-bit RSA
	if key.N.BitLen() != 2048 {
		t.Errorf("expected 2048-bit key, got %d", key.N.BitLen())
	}
}

func TestRSAEncryptDecryptRoundTrip(t *testing.T) {
	privKey, err := loadRSAPrivateKey()
	if err != nil {
		t.Fatalf("loadRSAPrivateKey failed: %v", err)
	}

	// Simulate what iOS does: encrypt a 16-byte AES key with the RSA public key
	// using OAEP, then decrypt with the private key
	aesKey := make([]byte, 16)
	if _, err := rand.Read(aesKey); err != nil {
		t.Fatalf("rand.Read failed: %v", err)
	}

	// Encrypt with public key (OAEP with SHA-1, like RAOP)
	encrypted, err := rsa.EncryptOAEP(sha1.New(), rand.Reader, &privKey.PublicKey, aesKey, nil)
	if err != nil {
		t.Fatalf("rsa.EncryptOAEP failed: %v", err)
	}

	// Decrypt with private key
	decrypted, err := rsa.DecryptOAEP(sha1.New(), rand.Reader, privKey, encrypted, nil)
	if err != nil {
		t.Fatalf("rsa.DecryptOAEP failed: %v", err)
	}

	if len(decrypted) != 16 {
		t.Errorf("expected 16 bytes, got %d", len(decrypted))
	}
	for i := range aesKey {
		if decrypted[i] != aesKey[i] {
			t.Fatalf(
				"decrypted key mismatch at byte %d: got %x, want %x",
				i,
				decrypted[i],
				aesKey[i],
			)
		}
	}
}

func TestRSASignVerifyRoundTrip(t *testing.T) {
	privKey, err := loadRSAPrivateKey()
	if err != nil {
		t.Fatalf("loadRSAPrivateKey failed: %v", err)
	}

	// Simulate the Apple-Challenge: 16 bytes padded to 32
	challenge := make([]byte, 16)
	if _, err := rand.Read(challenge); err != nil {
		t.Fatalf("rand.Read failed: %v", err)
	}
	padded := make([]byte, 32)
	copy(padded, challenge)

	// Sign with private key (PKCS#1 v1.5, raw — no hash, like RAOP)
	signed, err := rsa.SignPKCS1v15(rand.Reader, privKey, crypto.Hash(0), padded)
	if err != nil {
		t.Fatalf("rsa.SignPKCS1v15 failed: %v", err)
	}

	// Verify with public key
	if err := rsa.VerifyPKCS1v15(&privKey.PublicKey, crypto.Hash(0), padded, signed); err != nil {
		t.Fatalf("rsa.VerifyPKCS1v15 failed: %v", err)
	}
}

// --- Base64 Padding Tests ---

func TestPadBase64(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"YWJjZA", "YWJjZA=="},
		{"YWJjZGU", "YWJjZGU="},
		{"YWJjZGVm", "YWJjZGVm"},
		{"", ""},
		{"YWJj", "YWJj"},
	}

	for _, tt := range tests {
		got := padBase64(tt.input)
		if got != tt.expected {
			t.Errorf("padBase64(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestPadBase64RoundTrip(t *testing.T) {
	original := []byte("hello world this is a test")
	encoded := base64.StdEncoding.EncodeToString(original)
	stripped := strings.TrimRight(encoded, "=")

	padded := padBase64(stripped)
	decoded, err := base64.StdEncoding.DecodeString(padded)
	if err != nil {
		t.Fatalf("base64 decode failed: %v", err)
	}
	if string(decoded) != string(original) {
		t.Errorf("round-trip mismatch: got %q, want %q", decoded, original)
	}
}

// --- SDP Parsing Tests ---

func TestParseSDP(t *testing.T) {
	sdp := []byte("v=0\r\n" +
		"o=iTunes 3413821438 0 IN IP4 fe80::217:f2ff:fe0f:e0f6\r\n" +
		"s=iTunes\r\n" +
		"c=IN IP4 fe80::5a55:caff:fe1a:e187\r\n" +
		"t=0 0\r\n" +
		"m=audio 0 RTP/AVP 96\r\n" +
		"a=rtpmap:96 AppleLossless\r\n" +
		"a=fmtp:96 352 0 16 40 10 14 2 255 0 0 44100\r\n" +
		"a=rsaaeskey:5QYIqmdZGTONY5SHjEJrqAhaa0W9wzDC5i6q221mdGZJ5ubO6Kg\r\n" +
		"yhC6U83wpY87TFdPRdfPQl2kVC7+Uefmx1bXdIUo07ZcJsqMbgtje4w2JQw0b\r\n" +
		"a=aesiv:5b+YZi9Ikb845BmNhaVo+Q\r\n")

	result := parseSDP(sdp)

	// rtpmap includes payload type prefix
	if result["rtpmap"] != "96 AppleLossless" {
		t.Errorf("rtpmap = %q, want %q", result["rtpmap"], "96 AppleLossless")
	}
	// fmtp includes payload type prefix
	if result["fmtp"] != "96 352 0 16 40 10 14 2 255 0 0 44100" {
		t.Errorf("fmtp = %q, want %q", result["fmtp"], "96 352 0 16 40 10 14 2 255 0 0 44100")
	}
	if result["aesiv"] != "5b+YZi9Ikb845BmNhaVo+Q" {
		t.Errorf("aesiv = %q, want %q", result["aesiv"], "5b+YZi9Ikb845BmNhaVo+Q")
	}

	// rsaaeskey should be concatenated from continuation lines
	expectedKey := "5QYIqmdZGTONY5SHjEJrqAhaa0W9wzDC5i6q221mdGZJ5ubO6KgyhC6U83wpY87TFdPRdfPQl2kVC7+Uefmx1bXdIUo07ZcJsqMbgtje4w2JQw0b"
	if result["rsaaeskey"] != expectedKey {
		t.Errorf("rsaaeskey = %q, want %q", result["rsaaeskey"], expectedKey)
	}
}

func TestParseSDPWithWhitespace(t *testing.T) {
	// Test that whitespace in rsaaeskey continuation lines is properly joined
	sdp := []byte("a=rsaaeskey:5QYIqmdZGTONY5SHjEJrqAhaa0W9wzDC5i6q221mdGZJ5ubO6Kg\r\n" +
		"            yhC6U83wpY87TFdPRdfPQl2kVC7+Uefmx1bXdIUo07ZcJsqMbgtje4w2JQw0b\r\n" +
		"            Uw2BlzNPmVGQOxfdpGc3LXZzNE0jI1D4conUEiW6rrzikXBhk7Y/i2naw13ayy\r\n")

	result := parseSDP(sdp)
	key := result["rsaaeskey"]
	if strings.Contains(key, " ") {
		t.Errorf("rsaaeskey contains spaces: %q", key)
	}
	if strings.Contains(key, "\n") {
		t.Errorf("rsaaeskey contains newlines: %q", key)
	}
}

// --- Transport Port Parsing Tests ---

func TestParseTransportPorts(t *testing.T) {
	tests := []struct {
		transport   string
		wantAudio   int
		wantControl int
		wantTiming  int
	}{
		{
			transport:   "RTP/AVP/UDP;unicast;interleaved=0-1;mode=record;control_port=6001;timing_port=6002",
			wantControl: 6001,
			wantTiming:  6002,
		},
		{
			transport:   "RTP/AVP/UDP;unicast;mode=record;control_port=12345;timing_port=12346",
			wantControl: 12345,
			wantTiming:  12346,
		},
		{
			transport:   "RTP/AVP/UDP;unicast;client_port=50000-50001;mode=record;control_port=6001;timing_port=6002",
			wantAudio:   50000,
			wantControl: 6001,
			wantTiming:  6002,
		},
		{
			transport: "RTP/AVP/UDP;unicast;mode=record",
		},
	}

	for _, tt := range tests {
		audio, control, timing := parseTransportPorts(tt.transport)
		if audio != tt.wantAudio {
			t.Errorf("audio = %d, want %d (transport: %s)", audio, tt.wantAudio, tt.transport)
		}
		if control != tt.wantControl {
			t.Errorf("control = %d, want %d (transport: %s)", control, tt.wantControl, tt.transport)
		}
		if timing != tt.wantTiming {
			t.Errorf("timing = %d, want %d (transport: %s)", timing, tt.wantTiming, tt.transport)
		}
	}
}

// --- RTSP Request/Response Tests ---

func TestReadRTSPRequest(t *testing.T) {
	raw := "OPTIONS * RTSP/1.0\r\nCSeq: 1\r\nUser-Agent: AirPlay/1.0\r\n\r\n"
	req, err := readRTSPRequest(bufio.NewReader(strings.NewReader(raw)))
	if err != nil {
		t.Fatalf("readRTSPRequest failed: %v", err)
	}
	if req.method != "OPTIONS" {
		t.Errorf("method = %q, want OPTIONS", req.method)
	}
	if req.uri != "*" {
		t.Errorf("uri = %q, want *", req.uri)
	}
	if req.headers["CSeq"] != "1" {
		t.Errorf("CSeq = %q, want 1", req.headers["CSeq"])
	}
	if req.headers["User-Agent"] != "AirPlay/1.0" {
		t.Errorf("User-Agent = %q, want AirPlay/1.0", req.headers["User-Agent"])
	}
}

func TestReadRTSPRequestWithBody(t *testing.T) {
	body := "v=0\r\no=iTunes 0 0 IN IP4 0.0.0.0\r\n"
	raw := "ANNOUNCE rtsp://test/123 RTSP/1.0\r\nCSeq: 2\r\nContent-Type: application/sdp\r\nContent-Length: " +
		itoa(
			len(body),
		) + "\r\n\r\n" + body

	req, err := readRTSPRequest(bufio.NewReader(strings.NewReader(raw)))
	if err != nil {
		t.Fatalf("readRTSPRequest failed: %v", err)
	}
	if req.method != "ANNOUNCE" {
		t.Errorf("method = %q, want ANNOUNCE", req.method)
	}
	if string(req.body) != body {
		t.Errorf("body = %q, want %q", string(req.body), body)
	}
}

func TestWriteRTSPResponse(t *testing.T) {
	resp := &rtspResponse{
		status: 200,
		reason: "OK",
		headers: map[string]string{
			"CSeq":   "1",
			"Public": "ANNOUNCE, SETUP, RECORD",
		},
	}

	var buf strings.Builder
	if err := writeRTSPResponse(&buf, resp); err != nil {
		t.Fatalf("writeRTSPResponse failed: %v", err)
	}

	output := buf.String()
	if !strings.HasPrefix(output, "RTSP/1.0 200 OK\r\n") {
		t.Errorf("response doesn't start with status line: %q", output[:30])
	}
	if !strings.Contains(output, "CSeq: 1") {
		t.Errorf("response missing CSeq header: %q", output)
	}
	if !strings.Contains(output, "Public: ANNOUNCE, SETUP, RECORD") {
		t.Errorf("response missing Public header: %q", output)
	}
	if !strings.HasSuffix(output, "\r\n\r\n") {
		t.Errorf("response doesn't end with \\r\\n\\r\\n: %q", output[len(output)-10:])
	}
}

// --- OPTIONS Handler Test ---

func TestHandleOptions(t *testing.T) {
	server := newTestServer(t)
	req := &rtspRequest{
		method:  "OPTIONS",
		uri:     "*",
		headers: map[string]string{"CSeq": "42"},
	}

	resp := server.handleRequest(req)
	if resp.status != 200 {
		t.Errorf("OPTIONS status = %d, want 200", resp.status)
	}
	if resp.headers["CSeq"] != "42" {
		t.Errorf("CSeq = %q, want 42", resp.headers["CSeq"])
	}
	public := resp.headers["Public"]
	if !strings.Contains(public, "ANNOUNCE") {
		t.Errorf("Public header doesn't contain ANNOUNCE: %q", public)
	}
	if !strings.Contains(public, "SETUP") {
		t.Errorf("Public header doesn't contain SETUP: %q", public)
	}
	if !strings.Contains(public, "RECORD") {
		t.Errorf("Public header doesn't contain RECORD: %q", public)
	}
	if !strings.Contains(public, "TEARDOWN") {
		t.Errorf("Public header doesn't contain TEARDOWN: %q", public)
	}
}

// --- ANNOUNCE Handler Test (full RSA + AES handshake) ---

func TestHandleAnnounceWithRSAChallenge(t *testing.T) {
	privKey := mustLoadKey(t)
	server := newTestServer(t)

	// Generate a random AES key (what iOS would do)
	aesKey := make([]byte, 16)
	rand.Read(aesKey)

	// Encrypt AES key with RSA public key (OAEP, like iOS does)
	encryptedAesKey, err := rsa.EncryptOAEP(
		sha1.New(),
		rand.Reader,
		&privKey.PublicKey,
		aesKey,
		nil,
	)
	if err != nil {
		t.Fatalf("EncryptOAEP failed: %v", err)
	}
	rsaAesKeyB64 := strings.TrimRight(base64.StdEncoding.EncodeToString(encryptedAesKey), "=")

	// Generate random AES IV
	aesIV := make([]byte, 16)
	rand.Read(aesIV)
	aesIVB64 := strings.TrimRight(base64.StdEncoding.EncodeToString(aesIV), "=")

	// Generate Apple-Challenge (16 bytes)
	challenge := make([]byte, 16)
	rand.Read(challenge)
	challengeB64 := strings.TrimRight(base64.StdEncoding.EncodeToString(challenge), "=")

	// Build SDP
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

	req := &rtspRequest{
		method: "ANNOUNCE",
		uri:    "rtsp://test/123",
		headers: map[string]string{
			"CSeq":            "1",
			"Apple-Challenge": challengeB64,
		},
		body: []byte(sdp),
	}

	resp := server.handleRequest(req)
	if resp.status != 200 {
		t.Fatalf("ANNOUNCE status = %d, want 200. Response: %+v", resp.status, resp)
	}

	// Verify Apple-Response is present
	appleResponse := resp.headers["Apple-Response"]
	if appleResponse == "" {
		t.Fatal("Apple-Response header is missing")
	}

	// Verify the Apple-Response can be verified with the public key
	signedBytes, err := base64.StdEncoding.DecodeString(padBase64(appleResponse))
	if err != nil {
		t.Fatalf("decoding Apple-Response failed: %v", err)
	}

	// Pad challenge to 32 bytes (like the server does)
	padded := make([]byte, 32)
	copy(padded, challenge)

	// Verify signature (using crypto.Hash(0) = raw, no pre-hash)
	if err := rsa.VerifyPKCS1v15(&privKey.PublicKey, crypto.Hash(0), padded, signedBytes); err != nil {
		t.Errorf("Apple-Response signature verification failed: %v", err)
	}

	// Verify the session was created with the correct AES key
	server.sessionMu.Lock()
	sess := server.session
	server.sessionMu.Unlock()
	if sess == nil {
		t.Fatal("session was not created after ANNOUNCE")
	}
	if len(sess.aesKey) != 16 {
		t.Errorf("session AES key length = %d, want 16", len(sess.aesKey))
	}
	for i := range aesKey {
		if sess.aesKey[i] != aesKey[i] {
			t.Errorf("session AES key mismatch at byte %d", i)
		}
	}
	if len(sess.aesIV) != 16 {
		t.Errorf("session AES IV length = %d, want 16", len(sess.aesIV))
	}
	for i := range aesIV {
		if sess.aesIV[i] != aesIV[i] {
			t.Errorf("session AES IV mismatch at byte %d", i)
		}
	}
}

func TestHandleAnnounceNoChallenge(t *testing.T) {
	server := newTestServer(t)

	aesKey := make([]byte, 16)
	rand.Read(aesKey)
	encryptedAesKey, _ := rsa.EncryptOAEP(
		sha1.New(),
		rand.Reader,
		&server.rsaKey.PublicKey,
		aesKey,
		nil,
	)
	rsaAesKeyB64 := strings.TrimRight(base64.StdEncoding.EncodeToString(encryptedAesKey), "=")

	aesIV := make([]byte, 16)
	rand.Read(aesIV)
	aesIVB64 := strings.TrimRight(base64.StdEncoding.EncodeToString(aesIV), "=")

	sdp := "v=0\r\nm=audio 0 RTP/AVP 96\r\na=rsaaeskey:" + rsaAesKeyB64 + "\r\na=aesiv:" + aesIVB64 + "\r\n"

	req := &rtspRequest{
		method:  "ANNOUNCE",
		uri:     "rtsp://test/123",
		headers: map[string]string{"CSeq": "1"},
		body:    []byte(sdp),
	}

	resp := server.handleRequest(req)
	if resp.status != 200 {
		t.Fatalf("ANNOUNCE status = %d, want 200", resp.status)
	}
	if _, ok := resp.headers["Apple-Response"]; ok {
		t.Error("Apple-Response should not be present without Apple-Challenge")
	}
}

func TestHandleAnnounceBadKey(t *testing.T) {
	server := newTestServer(t)

	sdp := "v=0\r\nm=audio 0 RTP/AVP 96\r\na=rsaaeskey:INVALIDBASE64\r\na=aesiv:5b+YZi9Ikb845BmNhaVo+Q\r\n"

	req := &rtspRequest{
		method:  "ANNOUNCE",
		uri:     "rtsp://test/123",
		headers: map[string]string{"CSeq": "1"},
		body:    []byte(sdp),
	}

	resp := server.handleRequest(req)
	if resp.status != 400 {
		t.Errorf("ANNOUNCE with bad key should return 400, got %d", resp.status)
	}
}

// --- TEARDOWN/RECORD/SETUP without session ---

func TestHandleTeardownNoSession(t *testing.T) {
	server := newTestServer(t)
	req := &rtspRequest{
		method:  "TEARDOWN",
		uri:     "rtsp://test/123",
		headers: map[string]string{"CSeq": "1"},
	}

	resp := server.handleRequest(req)
	if resp.status != 200 {
		t.Errorf("TEARDOWN status = %d, want 200", resp.status)
	}
	if !resp.close {
		t.Error("TEARDOWN should set close=true")
	}
}

func TestHandleRecordNoSession(t *testing.T) {
	server := newTestServer(t)
	req := &rtspRequest{
		method:  "RECORD",
		uri:     "rtsp://test/123",
		headers: map[string]string{"CSeq": "1"},
	}

	resp := server.handleRequest(req)
	if resp.status != 454 {
		t.Errorf("RECORD without session should return 454, got %d", resp.status)
	}
}

func TestHandleSetupNoSession(t *testing.T) {
	server := newTestServer(t)
	req := &rtspRequest{
		method:  "SETUP",
		uri:     "rtsp://test/123",
		headers: map[string]string{"CSeq": "1"},
	}

	resp := server.handleRequest(req)
	if resp.status != 454 {
		t.Errorf("SETUP without session should return 454, got %d", resp.status)
	}
}

// --- AES Decryption Test (simulates audio packet decryption) ---

func TestAESDecryptionRoundTrip(t *testing.T) {
	aesKey := make([]byte, 16)
	rand.Read(aesKey)
	aesIV := make([]byte, 16)
	rand.Read(aesIV)

	// Create plaintext (ALAC frame, 16-byte aligned)
	plaintext := make([]byte, 64)
	rand.Read(plaintext)

	// Encrypt (simulating what iOS does)
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		t.Fatalf("aes.NewCipher failed: %v", err)
	}
	iv := make([]byte, 16)
	copy(iv, aesIV)
	encrypter := cipher.NewCBCEncrypter(block, iv)
	encrypted := make([]byte, len(plaintext))
	encrypter.CryptBlocks(encrypted, plaintext)

	// Decrypt (simulating what our server does)
	iv2 := make([]byte, 16)
	copy(iv2, aesIV)
	decrypter := cipher.NewCBCDecrypter(block, iv2)
	decrypted := make([]byte, len(encrypted))
	decrypter.CryptBlocks(decrypted, encrypted)

	for i := range plaintext {
		if decrypted[i] != plaintext[i] {
			t.Errorf(
				"decryption mismatch at byte %d: got %x, want %x",
				i,
				decrypted[i],
				plaintext[i],
			)
		}
	}
}

// --- ALAC Decoder Test ---

func TestAlacDecoderCreation(t *testing.T) {
	decoder, err := newAlacDecoder("352 0 16 40 10 14 2 255 0 0 44100")
	if err != nil {
		t.Fatalf("newAlacDecoder failed: %v", err)
	}
	if decoder == nil {
		t.Fatal("decoder is nil")
	}
}

// --- Helpers ---

func mustLoadKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := loadRSAPrivateKey()
	if err != nil {
		t.Fatalf("loadRSAPrivateKey failed: %v", err)
	}
	return key
}

// newTestServer creates a Server with a logger for testing.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	return &Server{
		rsaKey: mustLoadKey(t),
		log: clog.NewWithOptions(os.Stderr, clog.Options{
			Prefix:          "test-airplay",
			ReportTimestamp: false,
			Level:           clog.DebugLevel,
		}),
	}
}

// itoa is a simple int-to-string conversion to avoid strconv import in tests.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

package airplay

import (
	"bufio"
	"bytes"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alicebob/alac"
	clog "github.com/charmbracelet/log"
	"github.com/grandcat/zeroconf"
)

// Server is a RAOP (AirPlay v1) receiver that listens for AirPlay connections
// and routes received audio to a camera speaker.
type Server struct {
	name        string // AirPlay device name (shown in iOS AirPlay picker)
	port        int    // RTSP listener port
	hwAddr      string // fake MAC address for mDNS registration
	advertiseIP string // IP to advertise in mDNS (empty = auto-detect all interfaces)
	rsaKey      *rsa.PrivateKey
	edPriv      ed25519.PrivateKey // Ed25519 key for AirPlay pairing
	pkHex       string             // Ed25519 public key in hex (for pk= TXT record)
	piUUID      string             // Pairing identity UUID (for pi= TXT record)
	speaker     Speaker
	log         *clog.Logger
	listener    net.Listener
	zeroconf    *zeroconf.Server // RAOP _raop._tcp
	airplayZC   *zeroconf.Server // AirPlay _airplay._tcp

	// Active session
	sessionMu sync.Mutex
	session   *session
}

// Speaker is the interface for sending raw G.711ulaw audio to a camera.
// This matches cameras.Speaker but we define it locally to avoid import cycles.
type Speaker interface {
	SendRaw(rawFile string) error
	Stop() error
}

// NewServer creates a RAOP receiver for the given camera name.
// The name appears in the iOS AirPlay picker.
// advertiseIP is the IP address to advertise in mDNS (important for Docker host
// networking where bridge interfaces shouldn't be advertised). If empty, all
// interfaces are used.
func NewServer(name string, port int, advertiseIP string, speaker Speaker) (*Server, error) {
	key, err := loadRSAPrivateKey()
	if err != nil {
		return nil, fmt.Errorf("loading RSA key: %w", err)
	}

	// Generate a deterministic MAC address from the camera name so mDNS
	// entries stay stable across container restarts (random MACs leave
	// stale entries that confuse iOS).
	h := sha256.Sum256([]byte(name))
	hwAddr := fmt.Sprintf(
		"%02X%02X%02X%02X%02X%02X",
		h[0], h[1], h[2], h[3], h[4], h[5],
	)

	// Generate Ed25519 key pair for AirPlay pairing (pk= in mDNS).
	// Derive deterministically from camera name so it's stable across restarts.
	edSeed := sha256.Sum256([]byte("ed25519:" + name))
	edPriv := ed25519.NewKeyFromSeed(edSeed[:])
	edPub := edPriv.Public().(ed25519.PublicKey)
	pkHex := fmt.Sprintf("%x", edPub)
	piUUID := fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		binary.BigEndian.Uint32(h[0:4]),
		binary.BigEndian.Uint16(h[4:6]),
		binary.BigEndian.Uint16(h[6:8]),
		binary.BigEndian.Uint16(h[8:10]),
		h[10:16],
	)

	return &Server{
		name:        name,
		port:        port,
		hwAddr:      hwAddr,
		advertiseIP: advertiseIP,
		rsaKey:      key,
		edPriv:      edPriv,
		pkHex:       pkHex,
		piUUID:      piUUID,
		speaker:     speaker,
		log: clog.NewWithOptions(os.Stderr, clog.Options{
			Prefix:          fmt.Sprintf("airplay[%s]", name),
			ReportTimestamp: true,
			Level:           clog.InfoLevel,
		}),
	}, nil
}

// SetLogLevel changes the log level for this AirPlay server.
// Pass clog.DebugLevel for verbose protocol logging.
func (s *Server) SetLogLevel(level clog.Level) {
	s.log.SetLevel(level)
	s.log.SetReportCaller(false)
}

// Start begins listening for RAOP connections and advertising via mDNS.
func (s *Server) Start() error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("listening on port %d: %w", s.port, err)
	}
	s.listener = ln

	// Register via mDNS (Bonjour)
	// RAOP service name format: <MAC>@<display-name>
	raopName := fmt.Sprintf("%s@%s", s.hwAddr, s.name)
	text := []string{
		"txtvers=1",
		"ch=2",
		"cn=0,1", // PCM, ALAC
		"da=true",
		"et=0,1", // no encryption, RSA
		"ek=1",
		"md=0,1,2", // text, artwork, progress
		"pw=false",
		"sv=false",
		"sr=44100",
		"ss=16",
		"tp=UDP",
		"vn=65537",
		"vs=366.0",
		"am=camspeak",
		"sf=0x4",
		"ft=0x5A7FFEE6,0x0",
		"pk=" + s.pkHex,
		"vv=2",
	}

	var zc *zeroconf.Server
	if s.advertiseIP != "" {
		// Use RegisterProxy to advertise a specific IP — critical for Docker
		// host networking where bridge interfaces (172.x.x.x) must not be
		// advertised, only the LAN IP.
		// Note: zeroconf appends ".local." from the domain arg, so hostname
		// must NOT include ".local." — otherwise we get "name.local.local."
		hostname := s.name
		s.log.Debug("mDNS register", "mode", "proxy",
			"host", hostname, "ip", s.advertiseIP, "port", s.port)
		zc, err = zeroconf.RegisterProxy(
			raopName, "_raop._tcp", "local.",
			s.port, hostname, []string{s.advertiseIP}, text, nil,
		)
	} else {
		s.log.Debug("mDNS register", "mode", "auto", "port", s.port)
		zc, err = zeroconf.Register(raopName, "_raop._tcp", "local.", s.port, text, nil)
	}
	if err != nil {
		ln.Close()
		return fmt.Errorf("mDNS registration: %w", err)
	}
	s.zeroconf = zc

	// Also register _airplay._tcp — modern iOS requires both _raop._tcp
	// and _airplay._tcp to show the device in the AirPlay picker.
	// Minimal TXT records for audio-only AirPlay v1.
	airplayText := []string{
		"deviceid=" + formatMAC(s.hwAddr),
		"features=0x5A7FFEE6,0x0",
		"flags=0x4",
		"model=camspeak",
		"pw=false",
		"protovers=1.1",
		"srcvers=366.0",
		"vv=2",
		"pk=" + s.pkHex,
		"pi=" + s.piUUID,
		"gid=" + s.piUUID,
	}
	var airplayZC *zeroconf.Server
	if s.advertiseIP != "" {
		airplayZC, err = zeroconf.RegisterProxy(
			s.name, "_airplay._tcp", "local.",
			s.port, s.name, []string{s.advertiseIP}, airplayText, nil,
		)
	} else {
		airplayZC, err = zeroconf.Register(
			s.name, "_airplay._tcp", "local.", s.port, airplayText, nil,
		)
	}
	if err != nil {
		ln.Close()
		s.zeroconf.Shutdown()
		return fmt.Errorf("airplay mDNS registration: %w", err)
	}
	s.airplayZC = airplayZC

	s.log.Info("AirPlay receiver started", "port", s.port, "mDNS", raopName)

	go s.acceptLoop()

	return nil
}

// Stop shuts down the RAOP server.
func (s *Server) Stop() {
	if s.airplayZC != nil {
		s.airplayZC.Shutdown()
	}
	if s.zeroconf != nil {
		s.zeroconf.Shutdown()
	}
	if s.listener != nil {
		s.listener.Close()
	}
	s.sessionMu.Lock()
	if s.session != nil {
		s.session.teardown()
		s.session = nil
	}
	s.sessionMu.Unlock()
	s.log.Info("AirPlay receiver stopped")
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return // listener closed
		}
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	remote := conn.RemoteAddr().String()
	s.log.Debug("connection opened", "from", remote)

	reader := bufio.NewReader(conn)
	for {
		// Read the first line to determine if this is RTSP or HTTP
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				s.log.Debug("read error", "err", err, "from", remote)
			}
			s.log.Debug("connection closed", "from", remote)
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue // skip blank lines
		}

		// Check protocol: HTTP ends with HTTP/1.x, RTSP with RTSP/1.0
		if strings.Contains(line, "HTTP/1.") {
			// HTTP request — parse it and handle AirPlay endpoints
			if err := s.handleHTTPFromLine(reader, conn, line, remote); err != nil {
				s.log.Debug("HTTP connection closed", "err", err, "from", remote)
				return
			}
			continue
		}

		// RTSP request — parse from the line we already read
		req, err := parseRTSPFromLine(reader, line)
		if err != nil {
			s.log.Debug("RTSP parse error", "err", err, "from", remote)
			return
		}

		s.log.Debug("RTSP request", "method", req.method, "uri", req.uri,
			"CSeq", req.headers["CSeq"], "from", remote)

		resp := s.handleRequest(req)
		s.log.Debug("RTSP response", "status", resp.status,
			"CSeq", resp.headers["CSeq"], "from", remote)

		if err := writeRTSPResponse(conn, resp); err != nil {
			s.log.Debug("RTSP write error", "err", err)
			return
		}

		if resp.close {
			return
		}
	}
}

// handleHTTPFromLine processes an HTTP/1.x request from iOS, given that we've
// already read the first line (which contains method, URI, and HTTP/1.x).
func (s *Server) handleHTTPFromLine(
	r *bufio.Reader, conn net.Conn, firstLine string, remote string,
) error {
	parts := strings.SplitN(firstLine, " ", 3)
	if len(parts) < 3 {
		return fmt.Errorf("malformed HTTP request: %q", firstLine)
	}
	method := parts[0]
	uri := parts[1]

	// Read remaining headers
	headers := make(map[string]string)
	for {
		hline, err := r.ReadString('\n')
		if err != nil {
			return err
		}
		hline = strings.TrimSpace(hline)
		if hline == "" {
			break
		}
		kv := strings.SplitN(hline, ":", 2)
		if len(kv) == 2 {
			headers[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}

	// Read body if Content-Length present
	var body []byte
	if cl, ok := headers["Content-Length"]; ok {
		n, _ := strconv.Atoi(cl)
		if n > 0 {
			body = make([]byte, n)
			if _, err := io.ReadFull(r, body); err != nil {
				return err
			}
		}
	}

	s.log.Debug("HTTP request", "method", method, "uri", uri, "from", remote)

	// Handle AirPlay HTTP endpoints
	switch {
	case uri == "/info" || strings.HasPrefix(uri, "/info?"):
		resp := "HTTP/1.1 200 OK\r\nContent-Type: application/x-apple-binaryplist\r\nContent-Length: 0\r\n\r\n"
		_, err := conn.Write([]byte(resp))
		return err

	case uri == "/command" || strings.HasPrefix(uri, "/command?"):
		resp := "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"
		_, err := conn.Write([]byte(resp))
		return err

	case strings.HasPrefix(uri, "/pair-setup") || strings.HasPrefix(uri, "/pair-verify"):
		resp := "HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: 0\r\n\r\n"
		_, err := conn.Write([]byte(resp))
		return err

	default:
		s.log.Debug("HTTP unknown endpoint", "uri", uri, "method", method, "from", remote)
		resp := "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"
		_, err := conn.Write([]byte(resp))
		return err
	}
}

// parseRTSPFromLine parses an RTSP request when the first line has already
// been read from the reader.
func parseRTSPFromLine(r *bufio.Reader, firstLine string) (*rtspRequest, error) {
	parts := strings.SplitN(firstLine, " ", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("malformed RTSP request: %q", firstLine)
	}

	req := &rtspRequest{
		method:  parts[0],
		uri:     parts[1],
		headers: make(map[string]string),
	}

	// Read headers
	for {
		hline, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		hline = strings.TrimSpace(hline)
		if hline == "" {
			break
		}
		kv := strings.SplitN(hline, ":", 2)
		if len(kv) == 2 {
			req.headers[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}

	// Read body if Content-Length present
	if cl, ok := req.headers["Content-Length"]; ok {
		n, _ := strconv.Atoi(cl)
		if n > 0 {
			body := make([]byte, n)
			if _, err := io.ReadFull(r, body); err != nil {
				return nil, err
			}
			req.body = body
		}
	}

	return req, nil
}

// rtspRequest is a parsed RTSP request.
type rtspRequest struct {
	method  string
	uri     string
	headers map[string]string
	body    []byte
}

// rtspResponse is an RTSP response to send back.
type rtspResponse struct {
	status  int
	reason  string
	headers map[string]string
	body    []byte
	close   bool
}

// readRTSPRequest reads and parses a complete RTSP request from a reader.
// Used by tests. In production, handleConn reads the first line separately
// to detect HTTP vs RTSP.
func readRTSPRequest(r *bufio.Reader) (*rtspRequest, error) {
	// Read request line, skipping blank lines
	var line string
	for {
		l, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(l)
		if line != "" {
			break
		}
	}
	return parseRTSPFromLine(r, line)
}

func writeRTSPResponse(w io.Writer, resp *rtspResponse) error {
	if resp.headers == nil {
		resp.headers = make(map[string]string)
	}
	if _, ok := resp.headers["CSeq"]; !ok {
		resp.headers["CSeq"] = "0"
	}
	if _, ok := resp.headers["Server"]; !ok {
		resp.headers["Server"] = "AirTunes/366.0"
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "RTSP/1.0 %d %s\r\n", resp.status, resp.reason)
	for k, v := range resp.headers {
		fmt.Fprintf(&buf, "%s: %s\r\n", k, v)
	}
	if len(resp.body) > 0 {
		fmt.Fprintf(&buf, "Content-Length: %d\r\n", len(resp.body))
	}
	buf.WriteString("\r\n")
	if len(resp.body) > 0 {
		buf.Write(resp.body)
	}

	_, err := w.Write(buf.Bytes())
	return err
}

func (s *Server) handleRequest(req *rtspRequest) *rtspResponse {
	cseq := req.headers["CSeq"]

	switch req.method {
	case "OPTIONS":
		resp := &rtspResponse{
			status: 200,
			reason: "OK",
			headers: map[string]string{
				"CSeq":   cseq,
				"Public": "ANNOUNCE, SETUP, RECORD, PAUSE, FLUSH, TEARDOWN, OPTIONS, GET_PARAMETER, SET_PARAMETER, POST",
			},
		}
		// iOS sends Apple-Challenge with OPTIONS — must respond with Apple-Response
		if appleResp, ok := s.handleAppleChallenge(req); ok {
			resp.headers["Apple-Response"] = appleResp
		}
		return resp

	case "ANNOUNCE":
		return s.handleAnnounce(req, cseq)

	case "SETUP":
		return s.handleSetup(req, cseq)

	case "RECORD":
		return s.handleRecord(req, cseq)

	case "FLUSH":
		return s.handleFlush(req, cseq)

	case "TEARDOWN":
		return s.handleTeardown(req, cseq)

	case "SET_PARAMETER":
		// Volume control — accept but ignore
		return &rtspResponse{status: 200, reason: "OK", headers: map[string]string{"CSeq": cseq}}

	case "GET_PARAMETER":
		return &rtspResponse{status: 200, reason: "OK", headers: map[string]string{"CSeq": cseq}}

	case "POST":
		// POST /command and /feedback are control endpoints used by iOS
		// for metadata, volume, and playback feedback. Accept them with 200
		// so iOS proceeds with the RTSP ANNOUNCE/SETUP/RECORD flow.
		s.log.Debug("POST control endpoint", "uri", req.uri, "body_len", len(req.body))
		return &rtspResponse{status: 200, reason: "OK", headers: map[string]string{"CSeq": cseq}}

	default:
		return &rtspResponse{
			status:  405,
			reason:  "Method Not Allowed",
			headers: map[string]string{"CSeq": cseq},
		}
	}
}

// handleAppleChallenge processes the Apple-Challenge header from an RTSP
// request (sent with OPTIONS or ANNOUNCE) and returns the Apple-Response
// string (RSA-signed, base64-encoded). Returns ok=false if no challenge
// header is present.
func (s *Server) handleAppleChallenge(req *rtspRequest) (string, bool) {
	challenge, ok := req.headers["Apple-Challenge"]
	if !ok {
		return "", false
	}

	challengeBytes, err := base64.StdEncoding.DecodeString(padBase64(challenge))
	if err != nil {
		s.log.Warn("Apple-Challenge: bad base64", "err", err)
		return "", false
	}

	// Pad challenge to 32 bytes (RSA block size)
	padded := make([]byte, 32)
	copy(padded, challengeBytes)

	// Sign with RSA private key (PKCS#1 v1.5, raw — no hash)
	// RAOP uses RSA_private_encrypt with PKCS1_PADDING, which is equivalent
	// to SignPKCS1v15 with crypto.Hash(0) (no pre-hashing).
	signed, err := rsa.SignPKCS1v15(rand.Reader, s.rsaKey, crypto.Hash(0), padded)
	if err != nil {
		s.log.Warn("Apple-Challenge: RSA sign failed", "err", err)
		return "", false
	}

	resp := base64.StdEncoding.EncodeToString(signed)
	// Strip padding to match Apple's format
	return strings.TrimRight(resp, "="), true
}

// handleAnnounce parses the SDP, extracts AES key/IV, handles RSA challenge,
// and creates a new session.
func (s *Server) handleAnnounce(req *rtspRequest, cseq string) *rtspResponse {
	sdp := parseSDP(req.body)

	// Handle Apple-Challenge (RSA authentication) — may appear in ANNOUNCE too
	var appleResponse string
	if resp, ok := s.handleAppleChallenge(req); ok {
		appleResponse = resp
	}

	// Extract AES key from rsaaeskey (RSA-encrypted AES key)
	rsaAesKey, ok := sdp["rsaaeskey"]
	if !ok {
		s.log.Warn("ANNOUNCE: no rsaaeskey in SDP")
		return &rtspResponse{
			status:  400,
			reason:  "Bad Request",
			headers: map[string]string{"CSeq": cseq},
		}
	}

	// The rsaaeskey may have whitespace from line continuation
	rsaAesKey = strings.Join(strings.Fields(rsaAesKey), "")
	encryptedAesKey, err := base64.StdEncoding.DecodeString(padBase64(rsaAesKey))
	if err != nil {
		s.log.Warn("ANNOUNCE: bad rsaaeskey base64", "err", err)
		return &rtspResponse{
			status:  400,
			reason:  "Bad Request",
			headers: map[string]string{"CSeq": cseq},
		}
	}

	// Decrypt AES key with RSA private key (OAEP padding)
	aesKey, err := rsa.DecryptOAEP(sha1.New(), rand.Reader, s.rsaKey, encryptedAesKey, nil)
	if err != nil {
		s.log.Warn("ANNOUNCE: RSA decrypt failed", "err", err)
		return &rtspResponse{
			status:  400,
			reason:  "Bad Request",
			headers: map[string]string{"CSeq": cseq},
		}
	}

	if len(aesKey) != 16 {
		s.log.Warn("ANNOUNCE: unexpected AES key length", "len", len(aesKey))
		return &rtspResponse{
			status:  400,
			reason:  "Bad Request",
			headers: map[string]string{"CSeq": cseq},
		}
	}

	// Extract AES IV
	aesIVStr, ok := sdp["aesiv"]
	if !ok {
		s.log.Warn("ANNOUNCE: no aesiv in SDP")
		return &rtspResponse{
			status:  400,
			reason:  "Bad Request",
			headers: map[string]string{"CSeq": cseq},
		}
	}
	aesIVStr = strings.Join(strings.Fields(aesIVStr), "")
	aesIV, err := base64.StdEncoding.DecodeString(padBase64(aesIVStr))
	if err != nil || len(aesIV) != 16 {
		s.log.Warn("ANNOUNCE: bad aesiv", "err", err, "len", len(aesIV))
		return &rtspResponse{
			status:  400,
			reason:  "Bad Request",
			headers: map[string]string{"CSeq": cseq},
		}
	}

	// Parse fmtp for ALAC decoder config
	fmtp := sdp["fmtp"]
	rtpmap := sdp["rtpmap"]

	s.log.Info("ANNOUNCE received", "rtpmap", rtpmap, "fmtp", fmtp, "aesKeyLen", len(aesKey))

	// Create new session
	sess := &session{
		aesKey:  aesKey,
		aesIV:   aesIV,
		fmtp:    fmtp,
		log:     s.log,
		speaker: s.speaker,
	}

	// Initialize ALAC decoder
	if err := sess.initDecoder(); err != nil {
		s.log.Warn("ANNOUNCE: ALAC decoder init failed", "err", err)
		return &rtspResponse{
			status:  500,
			reason:  "Internal Error",
			headers: map[string]string{"CSeq": cseq},
		}
	}

	// Store session
	s.sessionMu.Lock()
	if s.session != nil {
		s.session.teardown()
	}
	s.session = sess
	s.sessionMu.Unlock()

	resp := &rtspResponse{
		status: 200,
		reason: "OK",
		headers: map[string]string{
			"CSeq":              cseq,
			"Audio-Jack-Status": "connected; type=analog",
		},
	}
	if appleResponse != "" {
		resp.headers["Apple-Response"] = appleResponse
	}
	return resp
}

// handleSetup allocates UDP ports for audio, control, and timing channels.
func (s *Server) handleSetup(req *rtspRequest, cseq string) *rtspResponse {
	s.sessionMu.Lock()
	sess := s.session
	s.sessionMu.Unlock()

	if sess == nil {
		return &rtspResponse{
			status:  454,
			reason:  "Session Not Found",
			headers: map[string]string{"CSeq": cseq},
		}
	}

	// Parse transport header to get client ports
	transport := req.headers["Transport"]
	clientAudioPort, clientControlPort, clientTimingPort := parseTransportPorts(transport)

	// Allocate UDP sockets
	audioPort, err := sess.setupAudioReceiver()
	if err != nil {
		s.log.Warn("SETUP: failed to allocate audio port", "err", err)
		return &rtspResponse{
			status:  500,
			reason:  "Internal Error",
			headers: map[string]string{"CSeq": cseq},
		}
	}

	controlPort, timingPort, err := sess.setupControlTiming()
	if err != nil {
		s.log.Warn("SETUP: failed to allocate control/timing ports", "err", err)
		return &rtspResponse{
			status:  500,
			reason:  "Internal Error",
			headers: map[string]string{"CSeq": cseq},
		}
	}

	// Store client ports for sync packets
	sess.clientAudioPort = clientAudioPort
	sess.clientControlPort = clientControlPort
	sess.clientTimingPort = clientTimingPort

	// Get client IP from the RTSP connection's remote addr (we need it from the session)
	// Actually, we get it from the SDP's connection info or from the TCP connection
	// For now, we'll get it from the SETUP request's URI
	if u, err := url.Parse(req.uri); err == nil {
		host := u.Hostname()
		if host != "" {
			sess.clientIP = host
		}
	}

	sess.sessionID = "1"

	s.log.Info(
		"SETUP done",
		"audioPort",
		audioPort,
		"controlPort",
		controlPort,
		"timingPort",
		timingPort,
		"clientAudio",
		clientAudioPort,
		"clientControl",
		clientControlPort,
		"clientTiming",
		clientTimingPort,
	)

	respTransport := fmt.Sprintf(
		"RTP/AVP/UDP;unicast;mode=record;server_port=%d;control_port=%d;timing_port=%d",
		audioPort,
		controlPort,
		timingPort,
	)

	return &rtspResponse{
		status: 200,
		reason: "OK",
		headers: map[string]string{
			"CSeq":              cseq,
			"Session":           sess.sessionID,
			"Transport":         respTransport,
			"Audio-Jack-Status": "connected; type=analog",
		},
	}
}

// handleRecord starts the audio streaming pipeline.
func (s *Server) handleRecord(req *rtspRequest, cseq string) *rtspResponse {
	s.sessionMu.Lock()
	sess := s.session
	s.sessionMu.Unlock()

	if sess == nil {
		return &rtspResponse{
			status:  454,
			reason:  "Session Not Found",
			headers: map[string]string{"CSeq": cseq},
		}
	}

	if err := sess.startStreaming(); err != nil {
		s.log.Warn("RECORD: failed to start streaming", "err", err)
		return &rtspResponse{
			status:  500,
			reason:  "Internal Error",
			headers: map[string]string{"CSeq": cseq},
		}
	}

	s.log.Info("RECORD — streaming started")

	return &rtspResponse{
		status: 200,
		reason: "OK",
		headers: map[string]string{
			"CSeq":          cseq,
			"Session":       sess.sessionID,
			"Audio-Latency": "2205",
		},
	}
}

// handleFlush stops the current streaming but keeps the session.
func (s *Server) handleFlush(req *rtspRequest, cseq string) *rtspResponse {
	s.sessionMu.Lock()
	sess := s.session
	s.sessionMu.Unlock()

	if sess == nil {
		return &rtspResponse{
			status:  454,
			reason:  "Session Not Found",
			headers: map[string]string{"CSeq": cseq},
		}
	}

	sess.flush()
	s.log.Info("FLUSH")

	return &rtspResponse{
		status:  200,
		reason:  "OK",
		headers: map[string]string{"CSeq": cseq, "Session": sess.sessionID},
	}
}

// handleTeardown ends the session and sends accumulated audio to the camera.
func (s *Server) handleTeardown(req *rtspRequest, cseq string) *rtspResponse {
	s.sessionMu.Lock()
	sess := s.session
	s.session = nil
	s.sessionMu.Unlock()

	if sess != nil {
		sess.teardown()
	}

	s.log.Info("TEARDOWN — session ended")

	return &rtspResponse{
		status:  200,
		reason:  "OK",
		headers: map[string]string{"CSeq": cseq},
		close:   true,
	}
}

// parseSDP extracts key SDP attributes into a map.
// Handles multi-line values (rsaaeskey can span multiple lines where
// continuation lines don't start with "a=").
func parseSDP(body []byte) map[string]string {
	sdp := make(map[string]string)
	lines := strings.Split(string(body), "\n")
	var lastKey string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			lastKey = ""
			continue
		}

		// Check if this is a standard SDP attribute line (starts with "X=")
		if len(line) > 2 && line[1] == '=' {
			// New attribute — reset lastKey
			if strings.HasPrefix(line, "a=") {
				kv := strings.SplitN(line[2:], ":", 2)
				if len(kv) == 2 {
					lastKey = strings.TrimSpace(kv[0])
					val := strings.TrimSpace(kv[1])
					sdp[lastKey] = val
				}
			} else {
				lastKey = ""
			}
		} else if lastKey != "" {
			// Continuation line — append to previous attribute
			sdp[lastKey] += line
		}
	}
	return sdp
}

// parseTransportPorts extracts client port numbers from the Transport header.
func parseTransportPorts(transport string) (audio, control, timing int) {
	// Example: RTP/AVP/UDP;unicast;interleaved=0-1;mode=record;control_port=6001;timing_port=6002
	parts := strings.Split(transport, ";")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		switch {
		case strings.HasPrefix(p, "control_port="):
			control, _ = strconv.Atoi(strings.TrimPrefix(p, "control_port="))
		case strings.HasPrefix(p, "timing_port="):
			timing, _ = strconv.Atoi(strings.TrimPrefix(p, "timing_port="))
		case strings.HasPrefix(p, "client_port="):
			ports := strings.Split(strings.TrimPrefix(p, "client_port="), "-")
			if len(ports) > 0 {
				audio, _ = strconv.Atoi(ports[0])
			}
		}
	}
	return
}

// padBase64 adds padding to a base64 string that may have had padding stripped.
func padBase64(s string) string {
	if r := len(s) % 4; r != 0 {
		s += strings.Repeat("=", 4-r)
	}
	return s
}

// formatMAC converts a hex MAC string "AABBCCDDEEFF" to "AA:BB:CC:DD:EE:FF".
func formatMAC(hw string) string {
	if len(hw) != 12 {
		return hw
	}
	return fmt.Sprintf("%s:%s:%s:%s:%s:%s",
		hw[0:2], hw[2:4], hw[4:6], hw[6:8], hw[8:10], hw[10:12])
}

// session holds the state of a single RAOP connection.
type session struct {
	aesKey  []byte
	aesIV   []byte
	fmtp    string
	log     *clog.Logger
	speaker Speaker

	sessionID         string
	clientIP          string
	clientAudioPort   int
	clientControlPort int
	clientTimingPort  int

	audioConn   *net.UDPConn
	controlConn *net.UDPConn
	timingConn  *net.UDPConn

	decoder *alacDecoder
	stream  *audioStream
	done    chan struct{}
}

// initDecoder creates an ALAC decoder from the fmtp parameters.
func (s *session) initDecoder() error {
	d, err := newAlacDecoder(s.fmtp)
	if err != nil {
		return err
	}
	s.decoder = d
	return nil
}

// setupAudioReceiver creates a UDP socket for receiving audio RTP packets.
func (s *session) setupAudioReceiver() (int, error) {
	addr, err := net.ResolveUDPAddr("udp4", ":0")
	if err != nil {
		return 0, err
	}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return 0, err
	}
	s.audioConn = conn
	return conn.LocalAddr().(*net.UDPAddr).Port, nil
}

// setupControlTiming creates UDP sockets for control and timing channels.
func (s *session) setupControlTiming() (int, int, error) {
	// Control port
	cAddr, _ := net.ResolveUDPAddr("udp4", ":0")
	cConn, err := net.ListenUDP("udp4", cAddr)
	if err != nil {
		return 0, 0, err
	}
	s.controlConn = cConn
	controlPort := cConn.LocalAddr().(*net.UDPAddr).Port

	// Timing port
	tAddr, _ := net.ResolveUDPAddr("udp4", ":0")
	tConn, err := net.ListenUDP("udp4", tAddr)
	if err != nil {
		cConn.Close()
		return 0, 0, err
	}
	s.timingConn = tConn
	timingPort := tConn.LocalAddr().(*net.UDPAddr).Port

	// Start timing and control listeners
	go s.timingLoop()
	go s.controlLoop()

	return controlPort, timingPort, nil
}

// startStreaming begins the audio receive → decode → transcode → camera pipeline.
func (s *session) startStreaming() error {
	stream, err := newAudioStream(s.speaker, s.log)
	if err != nil {
		return err
	}
	s.stream = stream
	s.done = make(chan struct{})

	go s.audioReceiveLoop()
	return nil
}

// audioReceiveLoop reads RTP packets, decrypts ALAC, decodes to PCM, and pipes to ffmpeg.
func (s *session) audioReceiveLoop() {
	buf := make([]byte, 16384)

	// Create AES cipher
	block, err := aes.NewCipher(s.aesKey)
	if err != nil {
		s.log.Warn("audio: AES cipher init failed", "err", err)
		return
	}

	for {
		select {
		case <-s.done:
			return
		default:
		}

		n, _, err := s.audioConn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				s.log.Debug("audio: read error", "err", err)
				continue
			}
		}

		if n < 12 {
			s.log.Debug("audio: packet too small", "len", n)
			continue // too small for RTP header
		}

		// Parse RTP header
		// Version (2 bits), Padding (1), Extension (1), CSRC count (4)
		// Marker (1), Payload type (7)
		// Sequence number (16)
		// Timestamp (32)
		// SSRC (32)
		payloadType := buf[1] & 0x7f
		if payloadType != 96 {
			s.log.Debug("audio: non-audio RTP packet", "payloadType", payloadType)
			continue // not audio
		}

		// RTP header is 12 bytes + 4*CSRC
		csrcCount := int(buf[0] & 0x0f)
		headerLen := 12 + 4*csrcCount
		if n < headerLen {
			continue
		}

		payload := buf[headerLen:n]
		if len(payload) == 0 {
			continue
		}

		seqNum := int(buf[2])<<8 | int(buf[3])
		s.log.Debug("audio: RTP packet",
			"seq", seqNum, "payloadLen", len(payload), "totalLen", n)

		// Decrypt with AES-128-CBC
		if len(payload)%16 != 0 {
			s.log.Debug("audio: payload not 16-byte aligned", "len", len(payload))
			continue
		}

		decrypted := make([]byte, len(payload))
		iv := make([]byte, 16)
		copy(iv, s.aesIV)
		mode := cipher.NewCBCDecrypter(block, iv)
		mode.CryptBlocks(decrypted, payload)

		// Decode ALAC frame → PCM 16-bit stereo 44100Hz
		pcm := s.decoder.Decode(decrypted)
		if len(pcm) == 0 {
			s.log.Debug("audio: ALAC decode returned empty", "encryptedLen", len(payload))
			continue
		}

		s.log.Debug("audio: decoded ALAC", "pcmLen", len(pcm), "seq", seqNum)

		// Feed PCM to the audio stream (which pipes to ffmpeg → G.711ulaw → camera)
		s.stream.writePCM(pcm)
	}
}

// timingLoop responds to NTP timing requests from the client.
func (s *session) timingLoop() {
	buf := make([]byte, 256)
	for {
		select {
		case <-s.done:
			return
		default:
		}

		n, addr, err := s.timingConn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if n < 8 {
			continue
		}

		// Timing request: RTP payload type 82
		// Timing reply: RTP payload type 83
		// Format: 8 bytes RTP header (no SSRC) + 8 bytes client NTP + 8 bytes server NTP + 4 bytes RTP time
		if buf[1]&0x7f != 82 {
			continue
		}

		// Build reply
		reply := make([]byte, 32)
		reply[0] = 0x80            // RTP version 2
		reply[1] = 83              // payload type 83 (timing reply)
		copy(reply[2:4], buf[2:4]) // sequence
		// Copy client timestamp
		copy(reply[8:16], buf[8:16])
		// Server NTP time (current time)
		now := time.Now().UnixNano()
		secs := uint32(now / 1e9)
		frac := uint32(uint64(now%1e9) * (uint64(1) << 32 / 1e9))
		binary.BigEndian.PutUint32(reply[16:20], secs+2208988800) // NTP epoch
		binary.BigEndian.PutUint32(reply[20:24], frac)
		// RTP time (same as client for now)
		copy(reply[24:28], buf[8:12])

		_, _ = s.timingConn.WriteToUDP(reply, addr)
	}
}

// controlLoop handles sync and retransmit packets from the client.
func (s *session) controlLoop() {
	buf := make([]byte, 256)
	for {
		select {
		case <-s.done:
			return
		default:
		}

		n, _, err := s.controlConn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		// We receive sync packets (type 84) and retransmit requests (type 85)
		// For now, just acknowledge them — we don't need precise sync for camera audio
		_ = n
	}
}

// flush stops the current audio stream but keeps the session alive.
func (s *session) flush() {
	if s.stream != nil {
		s.stream.flush()
	}
}

// teardown closes all connections and sends accumulated audio to the camera.
func (s *session) teardown() {
	if s.done != nil {
		select {
		case <-s.done:
		default:
			close(s.done)
		}
	}

	if s.audioConn != nil {
		s.audioConn.Close()
	}
	if s.controlConn != nil {
		s.controlConn.Close()
	}
	if s.timingConn != nil {
		s.timingConn.Close()
	}
	if s.stream != nil {
		s.stream.finish()
	}
}

// alacDecoder wraps the alicebob/alac decoder.
type alacDecoder struct {
	decoder *alac.Alac
}

// newAlacDecoder creates an ALAC decoder from the fmtp string.
// The fmtp format is: "352 0 16 40 10 14 2 255 0 0 44100"
func newAlacDecoder(fmtp string) (*alacDecoder, error) {
	// Use the alicebob/alac library which has sensible defaults for RAOP
	d, err := alac.New()
	if err != nil {
		return nil, fmt.Errorf("creating ALAC decoder: %w", err)
	}
	return &alacDecoder{decoder: d}, nil
}

// Decode decodes a single ALAC frame to 16-bit PCM.
func (d *alacDecoder) Decode(frame []byte) []byte {
	return d.decoder.Decode(frame)
}

// audioStream manages the pipeline: PCM → ffmpeg → G.711ulaw → temp file → camera.
// It accumulates audio in chunks and sends completed chunks to the camera.
type audioStream struct {
	speaker    Speaker
	log        *clog.Logger
	ffmpegCmd  *exec.Cmd
	ffmpegIn   io.WriteCloser
	ffmpegOut  io.ReadCloser
	rawFile    *os.File
	rawPath    string
	chunkCount int
	mu         sync.Mutex
}

// newAudioStream starts an ffmpeg process that converts PCM 44100Hz stereo
// to G.711ulaw 8000Hz mono, writing to a temp file.
func newAudioStream(speaker Speaker, log *clog.Logger) (*audioStream, error) {
	as := &audioStream{
		speaker: speaker,
		log:     log,
	}

	if err := as.startChunk(); err != nil {
		return nil, err
	}

	return as, nil
}

// startChunk begins a new ffmpeg process and temp file for the next audio chunk.
func (as *audioStream) startChunk() error {
	// Create temp file for G.711ulaw output
	tmpFile, err := os.CreateTemp("", "airplay-*.raw")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	as.rawFile = tmpFile
	as.rawPath = tmpFile.Name()

	// Start ffmpeg: PCM s16le 44100Hz stereo → G.711ulaw 8000Hz mono
	cmd := exec.Command(
		"ffmpeg",
		"-f", "s16le",
		"-ar", "44100",
		"-ac", "2",
		"-i", "pipe:0",
		"-ar", "8000",
		"-ac", "1",
		"-c:a", "pcm_mulaw",
		"-f", "mulaw",
		"pipe:1",
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		tmpFile.Close()
		os.Remove(as.rawPath)
		return fmt.Errorf("ffmpeg stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		tmpFile.Close()
		os.Remove(as.rawPath)
		return fmt.Errorf("ffmpeg stdout: %w", err)
	}
	cmd.Stderr = nil // suppress ffmpeg noise

	if err := cmd.Start(); err != nil {
		tmpFile.Close()
		os.Remove(as.rawPath)
		return fmt.Errorf("starting ffmpeg: %w", err)
	}

	as.ffmpegCmd = cmd
	as.ffmpegIn = stdin
	as.ffmpegOut = stdout

	// Goroutine to read ffmpeg output → temp file
	go func() {
		_, _ = io.Copy(as.rawFile, as.ffmpegOut)
	}()

	return nil
}

// writePCM feeds 16-bit PCM samples to the ffmpeg transcoder.
func (as *audioStream) writePCM(pcm []byte) {
	as.mu.Lock()
	defer as.mu.Unlock()
	if as.ffmpegIn != nil {
		as.ffmpegIn.Write(pcm) //nolint:errcheck // best-effort write
	}
}

// flush resets the stream for a new chunk (called on FLUSH).
func (as *audioStream) flush() {
	as.mu.Lock()
	defer as.mu.Unlock()
	// Close current chunk and send to camera
	as.closeChunk()
	// Start new chunk
	if err := as.startChunk(); err != nil {
		as.log.Warn("flush: failed to start new chunk", "err", err)
	}
}

// closeChunk finishes the current ffmpeg process and sends the audio to the camera.
func (as *audioStream) closeChunk() {
	if as.ffmpegIn != nil {
		as.ffmpegIn.Close()
		as.ffmpegIn = nil
	}
	if as.ffmpegCmd != nil {
		_ = as.ffmpegCmd.Wait()
		as.ffmpegCmd = nil
	}
	if as.rawFile != nil {
		as.rawFile.Close()
		as.rawFile = nil
	}

	// Check if we have audio to send
	info, err := os.Stat(as.rawPath)
	if err != nil || info.Size() == 0 {
		if as.rawPath != "" {
			os.Remove(as.rawPath)
		}
		return
	}

	as.chunkCount++
	as.log.Info(
		"sending audio chunk to camera",
		"chunk",
		as.chunkCount,
		"bytes",
		info.Size(),
		"file",
		as.rawPath,
	)

	// Send to camera in a goroutine (non-blocking)
	rawPath := as.rawPath
	go func() {
		if err := as.speaker.SendRaw(rawPath); err != nil {
			as.log.Warn("camera SendRaw failed", "err", err, "chunk", as.chunkCount)
		}
		os.Remove(rawPath)
	}()

	as.rawPath = ""
}

// finish closes the stream and sends the final chunk to the camera.
func (as *audioStream) finish() {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.closeChunk()
}

// ap-dev: standalone AirPlay (RAOP v1) receiver for debugging.
//
// Advertises via mDNS, accepts RTSP connections, decrypts ALAC audio, and
// saves received audio to a WAV file so you can verify end-to-end receipt.
// Every request/response is dumped verbosely so you can see exactly where
// iOS gets stuck.
//
// Usage:
//
//	go run ./ap-dev -name "TestAirPlay" -port 5100
//	go run ./ap-dev -name "TestAirPlay" -port 5100 -mode modern
//
// Flags:
//
//	-name   Device name shown in iOS AirPlay picker
//	-port   RTSP listener port
//	-out    Output WAV file path (default airplay-out.wav)
//	-play   Play WAV via afplay after receiving (macOS only)
//	-mode   "minimal" (classic Airport Express) or "modern" (with pk/pi)
//	-v      Very verbose: dump raw bytes of every packet
package main

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
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/alicebob/alac"
	"github.com/grandcat/zeroconf"
)

// --- AirPort Express RSA key (James Laird, 2011) ---

const airportExpressKey = `-----BEGIN RSA PRIVATE KEY-----
MIIEpQIBAAKCAQEA59dE8qLieItsH1WgjrcFRKj6eUWqi+bGLOX1HL3U3GhC/j0Qg90u3sG/1CUt
wC5vOYvfDmFI6oSFXi5ELabWJmT2dKHzBJKa3k9ok+8t9ucRqMd6DZHJ2YCCLlDRKSKv6kDqnw4U
wPdpOMXziC/AMj3Z/lUVX1G7WSHCAWKf1zNS1eLvqr+boEjXuBOitnZ/bDzPHrTOZz0Dew0uowxf
/+sG+NCK3eQJVxqcaJ/vEHKIVd2M+5qL71yJQ+87X6oV3eaYvt3zWZYD6z5vYTcrtij2VZ9Zmni/
UAaHqn9JdsBWLUEpVviYnhimNVvYFZeCXg/IdTQ+x4IRdiXNv5hEewIDAQABAoIBAQDl8Axy9XfW
BLmkzkEiqoSwF0PsmVrPzH9KsnwLGH+QZlvjWd8SWYGN7u1507HvhF5N3drJoVU3O14nDY4TFQAa
LlJ9VM35AApXaLyY1ERrN7u9ALKd2LUwYhM7Km539O4yUFYikE2nIPscEsA5ltpxOgUGCY7b7ez5
NtD6nL1ZKauw7aNXmVAvmJTcuPxWmoktF3gDJKK2wxZuNGcJE0uFQEG4Z3BrWP7yoNuSK3dii2jm
lpPHr0O/KnPQtzI3eguhe0TwUem/eYSdyzMyVx/YpwkzwtYL3sR5k0o9rKQLtvLzfAqdBxBurciz
aaA/L0HIgAmOit1GJA2saMxTVPNhAoGBAPfgv1oeZxgxmotiCcMXFEQEWflzhWYTsXrhUIuz5jFu
a39GLS99ZEErhLdrwj8rDDViRVJ5skOp9zFvlYAHs0xh92ji1E7V/ysnKBfsMrPkk5KSKPrnjndM
oPdevWnVkgJ5jxFuNgxkOLMuG9i53B4yMvDTCRiIPMQ++N2iLDaRAoGBAO9v//mU8eVkQaoANf0Z
oMjW8CN4xwWA2cSEIHkd9AfFkftuv8oyLDCG3ZAf0vrhrrtkrfa7ef+AUb69DNggq4mHQAYBp7L+
k5DKzJrKuO0r+R0YbY9pZD1+/g9dVt91d6LQNepUE/yY2PP5CNoFmjedpLHMOPFdVgqDzDFxU8hL
AoGBANDrr7xAJbqBjHVwIzQ4To9pb4BNeqDndk5Qe7fT3+/H1njGaC0/rXE0Qb7q5ySgnsCb3DvA
cJyRM9SJ7OKlGt0FMSdJD5KG0XPIpAVNwgpXXH5MDJg09KHeh0kXo+QA6viFBi21y340NonnEfdf
54PX4ZGS/Xac1UK+pLkBB+zRAoGAf0AY3H3qKS2lMEI4bzEFoHeK3G895pDaK3TFBVmD7fV0Zhov
17fegFPMwOII8MisYm9ZfT2Z0s5Ro3s5rkt+nvLAdfC/PYPKzTLalpGSwomSNYJcB9HNMlmhkGzc
1JnLYT4iyUyx6pcZBmCd8bD0iwY/FzcgNDaUmbX9+XDvRA0CgYEAkE7pIPlE71qvfJQgoA9em0gI
LAuE4Pu13aKiJnfft7hIjbK+5kyb3TysZvoyDnb3HOKvInK7vXbKuU4ISgxB2bB3HcYzQMGsz1qJ
2gG0N5hvJpzwwhbhXqFKA4zaaSrw622wDniAK5MlIE0tIAKKP4yxNGjoD2QYjhBGuhvkWKY=
-----END RSA PRIVATE KEY-----`

// --- Globals ---

var (
	verbose bool
	outFile string
	doPlay  bool
)

func main() {
	name := flag.String("name", "AirPlay-Test", "Device name in iOS AirPlay picker")
	port := flag.Int("port", 5100, "RTSP listener port")
	mode := flag.String("mode", "minimal", "Advertisement mode: minimal or modern")
	flag.StringVar(&outFile, "out", "airplay-out.wav", "Output WAV file")
	flag.BoolVar(&doPlay, "play", false, "Play via afplay after receiving (macOS)")
	flag.BoolVar(&verbose, "v", false, "Verbose: dump raw bytes")
	flag.Parse()

	rsaKey, err := loadRSAKey()
	if err != nil {
		log.Fatalf("RSA key: %v", err)
	}

	// Derive stable MAC and Ed25519 keys from name
	h := sha256.Sum256([]byte(*name))
	hwAddr := fmt.Sprintf("%02X%02X%02X%02X%02X%02X", h[0], h[1], h[2], h[3], h[4], h[5])

	edSeed := sha256.Sum256([]byte("ed25519:" + *name))
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

	srv := &server{
		name:   *name,
		port:   *port,
		hwAddr: hwAddr,
		pkHex:  pkHex,
		piUUID: piUUID,
		rsaKey: rsaKey,
		edPriv: edPriv,
		mode:   *mode,
	}

	if err := srv.start(); err != nil {
		log.Fatalf("start: %v", err)
	}

	log.Printf("=== AirPlay test receiver running ===")
	log.Printf("  Name : %s", *name)
	log.Printf("  Port : %d", *port)
	log.Printf("  Mode : %s", *mode)
	log.Printf("  MAC  : %s", formatMAC(hwAddr))
	log.Printf("  Out  : %s", outFile)
	log.Printf("")
	log.Printf("Open Control Center on your iPhone → AirPlay → look for '%s'", *name)
	log.Printf("")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	srv.stop()
	log.Println("Bye")
}

// --- Server ---

// fpState holds per-connection FairPlay state derived during /fp-setup.
type fpState struct {
	mode       int    // 0-3, from byte 14 of step-1 request
	step2Data  []byte // full 164-byte step-2 request from iOS
	sessionKey []byte // 16-byte derived session key (populated after step 2)
}

type server struct {
	name   string
	port   int
	hwAddr string
	pkHex  string
	piUUID string
	rsaKey *rsa.PrivateKey
	edPriv ed25519.PrivateKey
	mode   string

	listener  net.Listener
	zcRAOP    *zeroconf.Server
	zcAirPlay *zeroconf.Server

	mu      sync.Mutex
	session *session
	fp      fpState // FairPlay state for current connection
}

func (s *server) start() error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", s.port))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	s.listener = ln

	raopName := fmt.Sprintf("%s@%s", s.hwAddr, s.name)

	var txt []string
	if s.mode == "modern" {
		txt = s.modernTXT()
	} else {
		txt = s.minimalTXT()
	}

	log.Printf("[mDNS] registering _raop._tcp as %q", raopName)
	for _, t := range txt {
		log.Printf("[mDNS]   %s", t)
	}

	zc, err := zeroconf.Register(raopName, "_raop._tcp", "local.", s.port, txt, nil)
	if err != nil {
		ln.Close()
		return fmt.Errorf("mDNS _raop: %w", err)
	}
	s.zcRAOP = zc

	if s.mode == "modern" {
		airTXT := s.airplayTXT()
		log.Printf("[mDNS] registering _airplay._tcp as %q", s.name)
		for _, t := range airTXT {
			log.Printf("[mDNS]   %s", t)
		}
		airZC, err := zeroconf.Register(s.name, "_airplay._tcp", "local.", s.port, airTXT, nil)
		if err != nil {
			zc.Shutdown()
			ln.Close()
			return fmt.Errorf("mDNS _airplay: %w", err)
		}
		s.zcAirPlay = airZC
	}

	go s.acceptLoop()
	return nil
}

// minimalTXT returns classic Airport Express-style TXT records.
// No pk/pi/ft — as simple as possible.
func (s *server) minimalTXT() []string {
	return []string{
		"txtvers=1",
		"ch=2",   // 2 channels
		"cn=0,1", // PCM + ALAC (no AAC)
		"da=true",
		"et=0",     // RSA only, no FairPlay
		"md=0,1,2", // text, artwork, progress metadata
		"pw=false",
		"sv=false",
		"sr=44100",
		"ss=16",
		"tp=UDP",
		"vn=65537",
		"vs=130.14", // old Airport Express version
		"am=AirPort4,107",
		"sf=0x4",
	}
}

// modernTXT returns updated TXT records with Ed25519 key for iOS 14+.
func (s *server) modernTXT() []string {
	return []string{
		"txtvers=1",
		"ch=2",
		"cn=0,1",
		"da=true",
		"et=0",
		"md=0,1,2",
		"pw=false",
		"sv=false",
		"sr=44100",
		"ss=16",
		"tp=UDP",
		"vn=65537",
		"vs=366.0",
		"am=camspeak",
		"sf=0x4",
		"ft=0x4A7FFEE6,0x0",
		"pk=" + s.pkHex,
		"vv=2",
	}
}

func (s *server) airplayTXT() []string {
	return []string{
		"deviceid=" + formatMAC(s.hwAddr),
		"features=0x4A7FFEE6,0x0",
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
}

func (s *server) stop() {
	if s.zcAirPlay != nil {
		s.zcAirPlay.Shutdown()
	}
	if s.zcRAOP != nil {
		s.zcRAOP.Shutdown()
	}
	if s.listener != nil {
		s.listener.Close()
	}
	s.mu.Lock()
	if s.session != nil {
		s.session.teardown()
	}
	s.mu.Unlock()
}

func (s *server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		log.Printf("[conn] new connection from %s", conn.RemoteAddr())
		go s.handleConn(conn)
	}
}

func (s *server) handleConn(conn net.Conn) {
	remote := conn.RemoteAddr()
	lastMethod := "(none)"
	defer func() {
		conn.Close()
		log.Printf("[conn] closed %s after last method: %s", remote, lastMethod)
	}()

	r := bufio.NewReader(conn)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				log.Printf("[conn] read err from %s: %v (last method: %s)", remote, err, lastMethod)
			}
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Dispatch by protocol
		if strings.Contains(line, "HTTP/1.") {
			if err := s.handleHTTP(r, conn, line); err != nil {
				log.Printf("[http] error: %v", err)
				return
			}
			continue
		}

		req, err := parseRTSP(r, line)
		if err != nil {
			log.Printf("[rtsp] parse error: %v", err)
			return
		}

		lastMethod = req.method
		log.Printf("[rtsp] ← %s %s (CSeq=%s)", req.method, req.uri, req.headers["CSeq"])
		if verbose {
			for k, v := range req.headers {
				log.Printf("[rtsp]   %s: %s", k, v)
			}
			if len(req.body) > 0 {
				log.Printf("[rtsp]   body (%d bytes):\n%s", len(req.body), req.body)
			}
		}

		resp := s.dispatch(req, conn)

		log.Printf("[rtsp] → %d %s (CSeq=%s)", resp.status, resp.reason, resp.headers["CSeq"])
		if verbose {
			for k, v := range resp.headers {
				log.Printf("[rtsp]   %s: %s", k, v)
			}
		}

		if err := writeResp(conn, resp); err != nil {
			log.Printf("[rtsp] write error: %v", err)
			return
		}
		if resp.close {
			return
		}
	}
}

// --- HTTP handler (pair-setup, pair-verify, /info, /command, /fp-setup) ---

func (s *server) handleHTTP(r *bufio.Reader, conn net.Conn, firstLine string) error {
	parts := strings.SplitN(firstLine, " ", 3)
	if len(parts) < 3 {
		return fmt.Errorf("bad HTTP line: %q", firstLine)
	}
	method, uri := parts[0], parts[1]

	headers := make(map[string]string)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		kv := strings.SplitN(line, ":", 2)
		if len(kv) == 2 {
			headers[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	var body []byte
	if cl, ok := headers["Content-Length"]; ok {
		n, _ := strconv.Atoi(cl)
		if n > 0 {
			body = make([]byte, n)
			io.ReadFull(r, body) //nolint:errcheck
		}
	}

	log.Printf("[http] ← %s %s", method, uri)
	if verbose && len(body) > 0 {
		log.Printf("[http]   body (%d bytes): %x", len(body), body)
	}

	_, err := conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"))
	if err == nil {
		log.Printf("[http] → 200 OK")
	}
	return err
}

// --- RTSP dispatcher ---

func (s *server) dispatch(req *rtspReq, conn net.Conn) *rtspResp {
	cseq := req.headers["CSeq"]

	switch req.method {
	case "OPTIONS":
		resp := &rtspResp{
			status: 200, reason: "OK",
			headers: map[string]string{
				"CSeq":              cseq,
				"Public":            "ANNOUNCE, SETUP, RECORD, PAUSE, FLUSH, TEARDOWN, OPTIONS, GET_PARAMETER, SET_PARAMETER, POST",
				"Audio-Jack-Status": "connected; type=analog",
			},
		}
		if ar, ok := s.appleChallenge(req); ok {
			resp.headers["Apple-Response"] = ar
			log.Printf("[options] sent Apple-Response (RSA challenge)")
		} else {
			log.Printf("[options] no Apple-Challenge in request (iOS 18 behaviour)")
		}
		return resp

	case "ANNOUNCE":
		return s.handleAnnounce(req, cseq, conn)

	case "SETUP":
		return s.handleSetup(req, cseq)

	case "RECORD":
		return s.handleRecord(req, cseq)

	case "FLUSH":
		s.mu.Lock()
		sess := s.session
		s.mu.Unlock()
		if sess != nil {
			sess.flush()
		}
		return &rtspResp{status: 200, reason: "OK", headers: map[string]string{"CSeq": cseq}}

	case "TEARDOWN":
		s.mu.Lock()
		sess := s.session
		s.session = nil
		s.mu.Unlock()
		if sess != nil {
			sess.teardown()
		}
		log.Printf("[rtsp] TEARDOWN — session ended")
		return &rtspResp{
			status:  200,
			reason:  "OK",
			headers: map[string]string{"CSeq": cseq},
			close:   true,
		}

	case "SET_PARAMETER":
		log.Printf("[rtsp] SET_PARAMETER body: %s", req.body)
		return &rtspResp{status: 200, reason: "OK", headers: map[string]string{"CSeq": cseq}}

	case "GET_PARAMETER":
		var respBody string
		if strings.Contains(string(req.body), "volume") {
			respBody = "volume: -20.000000\r\n"
		}
		hdrs := map[string]string{"CSeq": cseq}
		if respBody != "" {
			hdrs["Content-Type"] = "text/parameters"
		}
		return &rtspResp{status: 200, reason: "OK", headers: hdrs, body: []byte(respBody)}

	case "POST":
		if strings.HasPrefix(req.uri, "/fp-setup") {
			log.Printf("[fp-setup] body len=%d, bytes=%x", len(req.body), req.body)
			resp, ok := s.fairplaySetupConn(req.body)
			if ok {
				return &rtspResp{
					status: 200, reason: "OK",
					headers: map[string]string{"CSeq": cseq, "Content-Type": "application/octet-stream"},
					body:    resp,
				}
			}
			return &rtspResp{status: 400, reason: "Bad Request", headers: map[string]string{"CSeq": cseq}}
		}
		return &rtspResp{status: 200, reason: "OK", headers: map[string]string{"CSeq": cseq}}

	default:
		log.Printf("[rtsp] unknown method: %s", req.method)
		return &rtspResp{
			status:  405,
			reason:  "Method Not Allowed",
			headers: map[string]string{"CSeq": cseq},
		}
	}
}

func (s *server) appleChallenge(req *rtspReq) (string, bool) {
	ch, ok := req.headers["Apple-Challenge"]
	if !ok {
		return "", false
	}
	cb, err := base64.StdEncoding.DecodeString(padB64(ch))
	if err != nil {
		log.Printf("[challenge] bad b64: %v", err)
		return "", false
	}
	padded := make([]byte, 32)
	copy(padded, cb)
	signed, err := rsa.SignPKCS1v15(rand.Reader, s.rsaKey, crypto.Hash(0), padded)
	if err != nil {
		log.Printf("[challenge] sign failed: %v", err)
		return "", false
	}
	return strings.TrimRight(base64.StdEncoding.EncodeToString(signed), "="), true
}

func (s *server) handleAnnounce(req *rtspReq, cseq string, conn net.Conn) *rtspResp {
	log.Printf("[announce] SDP:\n%s", req.body)

	sdp := parseSDP(req.body)

	var appleResp string
	if ar, ok := s.appleChallenge(req); ok {
		appleResp = ar
	}

	var aesKey []byte

	if fpKeyB64, ok := sdp["fpaeskey"]; ok {
		// iOS 18 FairPlay mode: audio AES key encrypted with FP session key
		log.Printf("[announce] FairPlay mode (fpaeskey present)")
		fpKeyB64 = strings.Join(strings.Fields(fpKeyB64), "")
		fpBlob, err := base64.StdEncoding.DecodeString(padB64(fpKeyB64))
		if err != nil {
			log.Printf("[announce] ERROR: bad fpaeskey b64: %v", err)
			return bad(cseq)
		}
		log.Printf("[announce] fpaeskey blob (%d bytes): %x", len(fpBlob), fpBlob)

		s.mu.Lock()
		sessionKey := s.fp.sessionKey
		s.mu.Unlock()

		if sessionKey == nil {
			log.Printf("[announce] ERROR: no FairPlay session key — fp-setup not complete")
			return bad(cseq)
		}

		var err2 error
		aesKey, err2 = decryptFPAESKey(fpBlob, sessionKey)
		if err2 != nil {
			log.Printf("[announce] ERROR: fpaeskey decrypt failed: %v", err2)
			return bad(cseq)
		}
		log.Printf("[announce] FairPlay AES key decrypted OK: %x", aesKey)

	} else if rsaKeyB64, ok := sdp["rsaaeskey"]; ok {
		// Legacy RSA mode
		log.Printf("[announce] RSA mode (rsaaeskey present)")
		rsaKeyB64 = strings.Join(strings.Fields(rsaKeyB64), "")
		encKey, err := base64.StdEncoding.DecodeString(padB64(rsaKeyB64))
		if err != nil {
			log.Printf("[announce] ERROR: bad rsaaeskey b64: %v", err)
			return bad(cseq)
		}
		aesKey, err = rsa.DecryptOAEP(sha1.New(), rand.Reader, s.rsaKey, encKey, nil)
		if err != nil {
			log.Printf("[announce] ERROR: RSA decrypt failed: %v", err)
			return bad(cseq)
		}
		log.Printf("[announce] RSA AES key decrypted OK: %x", aesKey)
	} else {
		log.Printf("[announce] ERROR: no rsaaeskey or fpaeskey in SDP")
		return bad(cseq)
	}

	if len(aesKey) != 16 {
		log.Printf("[announce] ERROR: unexpected AES key length %d", len(aesKey))
		return bad(cseq)
	}

	aesIVStr, ok := sdp["aesiv"]
	if !ok {
		log.Printf("[announce] ERROR: no aesiv in SDP")
		return bad(cseq)
	}
	aesIVStr = strings.Join(strings.Fields(aesIVStr), "")
	aesIV, err := base64.StdEncoding.DecodeString(padB64(aesIVStr))
	if err != nil || len(aesIV) != 16 {
		log.Printf("[announce] ERROR: bad aesiv: %v len=%d", err, len(aesIV))
		return bad(cseq)
	}

	log.Printf("[announce] AES key OK (%d bytes), IV OK (%d bytes)", len(aesKey), len(aesIV))
	log.Printf("[announce] rtpmap=%s fmtp=%s", sdp["rtpmap"], sdp["fmtp"])

	// Get client IP from the connection
	clientIP := conn.RemoteAddr().(*net.TCPAddr).IP.String()

	sess := &session{
		aesKey:   aesKey,
		aesIV:    aesIV,
		fmtp:     sdp["fmtp"],
		clientIP: clientIP,
	}
	if err := sess.init(); err != nil {
		log.Printf("[announce] decoder init failed: %v", err)
		return internalErr(cseq)
	}

	s.mu.Lock()
	if s.session != nil {
		s.session.teardown()
	}
	s.session = sess
	s.mu.Unlock()

	headers := map[string]string{
		"CSeq":              cseq,
		"Audio-Jack-Status": "connected; type=analog",
	}
	if appleResp != "" {
		headers["Apple-Response"] = appleResp
	}
	return &rtspResp{status: 200, reason: "OK", headers: headers}
}

func (s *server) handleSetup(req *rtspReq, cseq string) *rtspResp {
	s.mu.Lock()
	sess := s.session
	s.mu.Unlock()
	if sess == nil {
		return &rtspResp{
			status:  454,
			reason:  "Session Not Found",
			headers: map[string]string{"CSeq": cseq},
		}
	}

	transport := req.headers["Transport"]
	log.Printf("[setup] Transport: %s", transport)
	clientAudio, clientCtrl, clientTiming := parseTransport(transport)

	audioPort, err := sess.setupAudio()
	if err != nil {
		log.Printf("[setup] audio port: %v", err)
		return internalErr(cseq)
	}
	ctrlPort, timingPort, err := sess.setupCtrlTiming()
	if err != nil {
		log.Printf("[setup] ctrl/timing ports: %v", err)
		return internalErr(cseq)
	}

	sess.clientAudioPort = clientAudio
	sess.clientCtrlPort = clientCtrl
	sess.clientTimingPort = clientTiming

	log.Printf("[setup] server ports: audio=%d ctrl=%d timing=%d", audioPort, ctrlPort, timingPort)
	log.Printf(
		"[setup] client ports: audio=%d ctrl=%d timing=%d",
		clientAudio,
		clientCtrl,
		clientTiming,
	)

	respTransport := fmt.Sprintf(
		"RTP/AVP/UDP;unicast;mode=record;server_port=%d;control_port=%d;timing_port=%d",
		audioPort, ctrlPort, timingPort,
	)
	return &rtspResp{
		status: 200, reason: "OK",
		headers: map[string]string{
			"CSeq":              cseq,
			"Session":           "1",
			"Transport":         respTransport,
			"Audio-Jack-Status": "connected; type=analog",
		},
	}
}

func (s *server) handleRecord(req *rtspReq, cseq string) *rtspResp {
	s.mu.Lock()
	sess := s.session
	s.mu.Unlock()
	if sess == nil {
		return &rtspResp{
			status:  454,
			reason:  "Session Not Found",
			headers: map[string]string{"CSeq": cseq},
		}
	}
	sess.startStreaming()
	log.Printf("[record] streaming started — play audio from your iPhone now")
	return &rtspResp{
		status: 200, reason: "OK",
		headers: map[string]string{
			"CSeq":          cseq,
			"Session":       "1",
			"Audio-Latency": "2205",
		},
	}
}

// --- Session ---

type session struct {
	aesKey           []byte
	aesIV            []byte
	fmtp             string
	clientIP         string
	clientAudioPort  int
	clientCtrlPort   int
	clientTimingPort int

	decoder    *alac.Alac
	audioConn  *net.UDPConn
	ctrlConn   *net.UDPConn
	timingConn *net.UDPConn
	done       chan struct{}

	mu     sync.Mutex
	pcmBuf []byte // accumulated PCM
}

func (s *session) init() error {
	d, err := alac.New()
	if err != nil {
		return fmt.Errorf("alac.New: %w", err)
	}
	s.decoder = d
	return nil
}

func (s *session) setupAudio() (int, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return 0, err
	}
	s.audioConn = conn
	return conn.LocalAddr().(*net.UDPAddr).Port, nil
}

func (s *session) setupCtrlTiming() (int, int, error) {
	cConn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return 0, 0, err
	}
	tConn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		cConn.Close()
		return 0, 0, err
	}
	s.ctrlConn = cConn
	s.timingConn = tConn
	go s.timingLoop()
	go s.ctrlLoop()
	return cConn.LocalAddr().(*net.UDPAddr).Port, tConn.LocalAddr().(*net.UDPAddr).Port, nil
}

func (s *session) startStreaming() {
	s.done = make(chan struct{})
	go s.audioLoop()
}

func (s *session) flush() {
	// Nothing to flush in this simple implementation
}

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
	if s.ctrlConn != nil {
		s.ctrlConn.Close()
	}
	if s.timingConn != nil {
		s.timingConn.Close()
	}

	s.mu.Lock()
	pcm := s.pcmBuf
	s.mu.Unlock()

	if len(pcm) == 0 {
		log.Printf("[session] teardown — no audio received")
		return
	}

	log.Printf("[session] teardown — %d PCM bytes received, saving...", len(pcm))
	if err := saveWAV(outFile, pcm, 44100, 2, 16); err != nil {
		log.Printf("[session] ERROR saving WAV: %v", err)
		return
	}
	log.Printf("[session] saved to %s", outFile)

	if doPlay {
		log.Printf("[session] playing via afplay...")
		cmd := exec.Command("afplay", outFile)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Printf("[session] afplay error: %v", err)
		}
	}
}

func (s *session) audioLoop() {
	block, err := aes.NewCipher(s.aesKey)
	if err != nil {
		log.Printf("[audio] AES cipher: %v", err)
		return
	}

	buf := make([]byte, 16384)
	pktCount := 0
	decodeOK := 0

	for {
		select {
		case <-s.done:
			log.Printf("[audio] loop stopped: %d packets received, %d decoded OK", pktCount, decodeOK)
			return
		default:
		}

		_ = s.audioConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, addr, err := s.audioConn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			select {
			case <-s.done:
				return
			default:
				log.Printf("[audio] read error: %v", err)
				continue
			}
		}

		pktCount++
		if pktCount == 1 {
			log.Printf("[audio] first packet from %s (%d bytes) !", addr, n)
		}
		if verbose {
			log.Printf("[audio] pkt #%d from %s: %d bytes", pktCount, addr, n)
		}

		if n < 12 {
			log.Printf("[audio] pkt too small: %d", n)
			continue
		}

		pt := buf[1] & 0x7f
		if pt != 96 {
			log.Printf("[audio] non-audio RTP payload type %d", pt)
			continue
		}

		csrc := int(buf[0] & 0x0f)
		hdr := 12 + 4*csrc
		if n < hdr {
			continue
		}
		payload := buf[hdr:n]
		if len(payload) == 0 || len(payload)%16 != 0 {
			log.Printf("[audio] payload not 16-byte aligned: %d", len(payload))
			continue
		}

		// Decrypt AES-128-CBC
		dec := make([]byte, len(payload))
		iv := make([]byte, 16)
		copy(iv, s.aesIV)
		cipher.NewCBCDecrypter(block, iv).CryptBlocks(dec, payload)

		// Decode ALAC → PCM
		pcm := s.decoder.Decode(dec)
		if len(pcm) == 0 {
			log.Printf("[audio] ALAC decode empty (encLen=%d)", len(payload))
			continue
		}
		decodeOK++

		s.mu.Lock()
		s.pcmBuf = append(s.pcmBuf, pcm...)
		s.mu.Unlock()
	}
}

func (s *session) timingLoop() {
	buf := make([]byte, 256)
	for {
		select {
		case <-s.done:
			return
		default:
		}
		_ = s.timingConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, addr, err := s.timingConn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return
		}
		if n < 8 || buf[1]&0x7f != 82 {
			continue
		}
		reply := make([]byte, 32)
		reply[0] = 0x80
		reply[1] = 83
		copy(reply[2:4], buf[2:4])
		copy(reply[8:16], buf[8:16])
		now := time.Now().UnixNano()
		binary.BigEndian.PutUint32(reply[16:20], uint32(now/1e9)+2208988800)
		binary.BigEndian.PutUint32(reply[20:24], uint32(uint64(now%1e9)*(uint64(1)<<32/1e9)))
		copy(reply[24:28], buf[8:12])
		s.timingConn.WriteToUDP(reply, addr) //nolint:errcheck
	}
}

func (s *session) ctrlLoop() {
	buf := make([]byte, 256)
	for {
		select {
		case <-s.done:
			return
		default:
		}
		_ = s.ctrlConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, _, err := s.ctrlConn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			return
		}
	}
}

// --- WAV ---

// mustWriteBin writes a binary value to a bytes.Buffer. bytes.Buffer.Write never
// returns an error, so this helper panics if one ever appears (it won't).
func mustWriteBin(w *bytes.Buffer, order binary.ByteOrder, v any) {
	if err := binary.Write(w, order, v); err != nil {
		panic(err)
	}
}

func saveWAV(path string, pcm []byte, sampleRate, channels, bitsPerSample int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	dataSize := uint32(len(pcm))
	byteRate := uint32(sampleRate * channels * bitsPerSample / 8)
	blockAlign := uint16(channels * bitsPerSample / 8)

	var hdr bytes.Buffer
	hdr.WriteString("RIFF")
	mustWriteBin(&hdr, binary.LittleEndian, 36+dataSize)
	hdr.WriteString("WAVE")
	hdr.WriteString("fmt ")
	mustWriteBin(&hdr, binary.LittleEndian, uint32(16))
	mustWriteBin(&hdr, binary.LittleEndian, uint16(1)) // PCM
	mustWriteBin(&hdr, binary.LittleEndian, uint16(channels))
	mustWriteBin(&hdr, binary.LittleEndian, uint32(sampleRate))
	mustWriteBin(&hdr, binary.LittleEndian, byteRate)
	mustWriteBin(&hdr, binary.LittleEndian, blockAlign)
	mustWriteBin(&hdr, binary.LittleEndian, uint16(bitsPerSample))
	hdr.WriteString("data")
	mustWriteBin(&hdr, binary.LittleEndian, dataSize)

	if _, err := f.Write(hdr.Bytes()); err != nil {
		return err
	}
	_, err = f.Write(pcm)
	return err
}

// --- FairPlay key derivation ---
//
// iOS 18 uses FairPlay to encrypt the audio AES key (fpaeskey instead of rsaaeskey).
// The encryption uses a per-session key derived from the /fp-setup handshake.
//
// Algorithm (from RPiPlay / UxPlay fair_play.c):
//   1. /fp-setup step 1: server returns one of 4 hardcoded 142-byte certificates
//      selected by mode byte at request[14].
//   2. /fp-setup step 2 (164 bytes): iOS sends encrypted session seed.
//      Server AES-128-ECB-decrypts request[12:28] using the mode-specific master key
//      to obtain the 16-byte session key, then echoes req[144:164] back wrapped in
//      the standard FPLY header.
//   3. ANNOUNCE SDP contains fpaeskey blob. Audio AES key is at blob[36:52],
//      encrypted with the session key (AES-128-ECB).

// fpMasterKeys are the 4 mode-specific AES-128 keys used to derive the session key.
// Source: RPiPlay/UxPlay fair_play.c (public domain, extracted from AirPort firmware).
var fpMasterKeys = [4][16]byte{
	{0x59, 0xD9, 0x0A, 0xE9, 0x26, 0xBF, 0x25, 0x7F, 0x02, 0x7E, 0x8F, 0x7D, 0xC4, 0x97, 0x15, 0x55},
	{0xF5, 0x26, 0xFE, 0x8F, 0xAA, 0x17, 0xEB, 0xFC, 0x0B, 0x56, 0xA8, 0x17, 0xB9, 0x38, 0x33, 0xD0},
	{0xF7, 0xD6, 0x9C, 0xE2, 0x09, 0x9C, 0x2B, 0x4C, 0x2E, 0x0A, 0xBB, 0xBE, 0x54, 0x67, 0x13, 0x02},
	{0x1B, 0x0B, 0x92, 0x98, 0x59, 0x2F, 0x8A, 0x2C, 0xDF, 0xB4, 0xD9, 0x4D, 0x16, 0x60, 0x7C, 0x9D},
}

// fpCertificates are the 4 mode-specific 142-byte server certificates returned in /fp-setup step 1.
// Source: RPiPlay/UxPlay fair_play.c (public domain).
var fpCertificates = [4][142]byte{
	{
		0x46,
		0x50,
		0x4c,
		0x59,
		0x03,
		0x01,
		0x02,
		0x00,
		0x00,
		0x00,
		0x00,
		0x82,
		0x02,
		0x00,
		0x0f,
		0x9f,
		0x3f,
		0x9e,
		0x0a,
		0x25,
		0x21,
		0xdb,
		0xdf,
		0x31,
		0x2a,
		0xb2,
		0xbf,
		0xb2,
		0x9e,
		0x8d,
		0x23,
		0x2b,
		0x63,
		0x76,
		0xa8,
		0xc8,
		0x18,
		0x70,
		0x1d,
		0x22,
		0xae,
		0x93,
		0xd8,
		0x27,
		0x37,
		0xfe,
		0xaf,
		0x9d,
		0xb4,
		0xfd,
		0xf4,
		0x1c,
		0x2d,
		0xba,
		0x9d,
		0x1f,
		0x49,
		0xca,
		0xaa,
		0xbf,
		0x65,
		0x91,
		0xac,
		0x1f,
		0x7b,
		0xc6,
		0xf7,
		0xe0,
		0x66,
		0x3d,
		0x21,
		0xaf,
		0xe0,
		0x15,
		0x65,
		0x95,
		0x3e,
		0xab,
		0x81,
		0xf4,
		0x18,
		0xce,
		0xed,
		0x09,
		0x5a,
		0xdb,
		0x7c,
		0x3d,
		0x0e,
		0x25,
		0x49,
		0x09,
		0xa7,
		0x98,
		0x31,
		0xd4,
		0x9c,
		0x39,
		0x82,
		0x97,
		0x34,
		0x34,
		0xfa,
		0xcb,
		0x42,
		0xc6,
		0x3a,
		0x1c,
		0xd9,
		0x11,
		0xa6,
		0xfe,
		0x94,
		0x1a,
		0x8a,
		0x6d,
		0x4a,
		0x74,
		0x3b,
		0x46,
		0xc3,
		0xa7,
		0x64,
		0x9e,
		0x44,
		0xc7,
		0x89,
		0x55,
		0xe4,
		0x9d,
		0x81,
		0x55,
		0x00,
		0x95,
		0x49,
		0xc4,
		0xe2,
		0xf7,
		0xa3,
		0xf6,
		0xd5,
		0xba,
	},
	{
		0x46,
		0x50,
		0x4c,
		0x59,
		0x03,
		0x01,
		0x02,
		0x00,
		0x00,
		0x00,
		0x00,
		0x82,
		0x02,
		0x01,
		0xcf,
		0x32,
		0xa2,
		0x57,
		0x14,
		0xb2,
		0x52,
		0x4f,
		0x8a,
		0xa0,
		0xad,
		0x7a,
		0xf1,
		0x64,
		0xe3,
		0x7b,
		0xcf,
		0x44,
		0x24,
		0xe2,
		0x00,
		0x04,
		0x7e,
		0xfc,
		0x0a,
		0xd6,
		0x7a,
		0xfc,
		0xd9,
		0x5d,
		0xed,
		0x1c,
		0x27,
		0x30,
		0xbb,
		0x59,
		0x1b,
		0x96,
		0x2e,
		0xd6,
		0x3a,
		0x9c,
		0x4d,
		0xed,
		0x88,
		0xba,
		0x8f,
		0xc7,
		0x8d,
		0xe6,
		0x4d,
		0x91,
		0xcc,
		0xfd,
		0x5c,
		0x7b,
		0x56,
		0xda,
		0x88,
		0xe3,
		0x1f,
		0x5c,
		0xce,
		0xaf,
		0xc7,
		0x43,
		0x19,
		0x95,
		0xa0,
		0x16,
		0x65,
		0xa5,
		0x4e,
		0x19,
		0x39,
		0xd2,
		0x5b,
		0x94,
		0xdb,
		0x64,
		0xb9,
		0xe4,
		0x5d,
		0x8d,
		0x06,
		0x3e,
		0x1e,
		0x6a,
		0xf0,
		0x7e,
		0x96,
		0x56,
		0x16,
		0x2b,
		0x0e,
		0xfa,
		0x40,
		0x42,
		0x75,
		0xea,
		0x5a,
		0x44,
		0xd9,
		0x59,
		0x1c,
		0x72,
		0x56,
		0xb9,
		0xfb,
		0xe6,
		0x51,
		0x38,
		0x98,
		0xb8,
		0x02,
		0x27,
		0x72,
		0x19,
		0x88,
		0x57,
		0x16,
		0x50,
		0x94,
		0x2a,
		0xd9,
		0x46,
		0x68,
		0x8a,
	},
	{
		0x46,
		0x50,
		0x4c,
		0x59,
		0x03,
		0x01,
		0x02,
		0x00,
		0x00,
		0x00,
		0x00,
		0x82,
		0x02,
		0x02,
		0xc1,
		0x69,
		0xa3,
		0x52,
		0xee,
		0xed,
		0x35,
		0xb1,
		0x8c,
		0xdd,
		0x9c,
		0x58,
		0xd6,
		0x4f,
		0x16,
		0xc1,
		0x51,
		0x9a,
		0x89,
		0xeb,
		0x53,
		0x17,
		0xbd,
		0x0d,
		0x43,
		0x36,
		0xcd,
		0x68,
		0xf6,
		0x38,
		0xff,
		0x9d,
		0x01,
		0x6a,
		0x5b,
		0x52,
		0xb7,
		0xfa,
		0x92,
		0x16,
		0xb2,
		0xb6,
		0x54,
		0x82,
		0xc7,
		0x84,
		0x44,
		0x11,
		0x81,
		0x21,
		0xa2,
		0xc7,
		0xfe,
		0xd8,
		0x3d,
		0xb7,
		0x11,
		0x9e,
		0x91,
		0x82,
		0xaa,
		0xd7,
		0xd1,
		0x8c,
		0x70,
		0x63,
		0xe2,
		0xa4,
		0x57,
		0x55,
		0x59,
		0x10,
		0xaf,
		0x9e,
		0x0e,
		0xfc,
		0x76,
		0x34,
		0x7d,
		0x16,
		0x40,
		0x43,
		0x80,
		0x7f,
		0x58,
		0x1e,
		0xe4,
		0xfb,
		0xe4,
		0x2c,
		0xa9,
		0xde,
		0xdc,
		0x1b,
		0x5e,
		0xb2,
		0xa3,
		0xaa,
		0x3d,
		0x2e,
		0xcd,
		0x59,
		0xe7,
		0xee,
		0xe7,
		0x0b,
		0x36,
		0x29,
		0xf2,
		0x2a,
		0xfd,
		0x16,
		0x1d,
		0x87,
		0x73,
		0x53,
		0xdd,
		0xb9,
		0x9a,
		0xdc,
		0x8e,
		0x07,
		0x00,
		0x6e,
		0x56,
		0xf8,
		0x50,
		0xce,
	},
	{
		0x46,
		0x50,
		0x4c,
		0x59,
		0x03,
		0x01,
		0x02,
		0x00,
		0x00,
		0x00,
		0x00,
		0x82,
		0x02,
		0x03,
		0x90,
		0x01,
		0xe1,
		0x72,
		0x7e,
		0x0f,
		0x57,
		0xf9,
		0xf5,
		0x88,
		0x0d,
		0xb1,
		0x04,
		0xa6,
		0x25,
		0x7a,
		0x23,
		0xf5,
		0xcf,
		0xff,
		0x1a,
		0xbb,
		0xe1,
		0xe9,
		0x30,
		0x45,
		0x25,
		0x1a,
		0xfb,
		0x97,
		0xeb,
		0x9f,
		0xc0,
		0x01,
		0x1e,
		0xbe,
		0x0f,
		0x3a,
		0x81,
		0xdf,
		0x5b,
		0x69,
		0x1d,
		0x76,
		0xac,
		0xb2,
		0xf7,
		0xa5,
		0xc7,
		0x08,
		0xe3,
		0xd3,
		0x28,
		0xf5,
		0x6b,
		0xb3,
		0x9d,
		0xbd,
		0xe5,
		0xf2,
		0x9c,
		0x8a,
		0x17,
		0xf4,
		0x81,
		0x48,
		0x7e,
		0x3a,
		0xe8,
		0x63,
		0xc6,
		0x78,
		0x32,
		0x54,
		0x22,
		0xe6,
		0xf7,
		0x8e,
		0x16,
		0x6d,
		0x18,
		0xaa,
		0x7f,
		0xd6,
		0x36,
		0x25,
		0x8b,
		0xce,
		0x28,
		0x72,
		0x6f,
		0x66,
		0x1f,
		0x73,
		0x88,
		0x93,
		0xce,
		0x44,
		0x31,
		0x1e,
		0x4b,
		0xe6,
		0xc0,
		0x53,
		0x51,
		0x93,
		0xe5,
		0xef,
		0x72,
		0xe8,
		0x68,
		0x62,
		0x33,
		0x72,
		0x9c,
		0x22,
		0x7d,
		0x82,
		0x0c,
		0x99,
		0x94,
		0x45,
		0xd8,
		0x92,
		0x46,
		0xc8,
		0xc3,
		0x59,
	},
}

var fpRespHeader = [12]byte{0x46, 0x50, 0x4c, 0x59, 0x03, 0x01, 0x04, 0x00, 0x00, 0x00, 0x00, 0x14}

// fairplaySetupConn handles /fp-setup and saves per-connection state.
func (s *server) fairplaySetupConn(body []byte) ([]byte, bool) {
	if len(body) < 16 || body[4] != 0x03 {
		log.Printf("[fp-setup] unexpected: len=%d body[4]=%02x", len(body), safeIdx(body, 4))
		return nil, false
	}

	if len(body) < 164 {
		// Step 1: return mode-specific certificate, save mode
		mode := int(body[14])
		if mode > 3 {
			log.Printf("[fp-setup] step 1: unknown mode %d", mode)
			return nil, false
		}
		s.mu.Lock()
		s.fp.mode = mode
		s.fp.sessionKey = nil
		s.mu.Unlock()
		log.Printf("[fp-setup] step 1: mode=%d", mode)
		reply := make([]byte, 142)
		copy(reply, fpCertificates[mode][:])
		return reply, true
	}

	// Step 2 (164 bytes): derive session key then return echo response
	s.mu.Lock()
	mode := s.fp.mode
	s.fp.step2Data = make([]byte, len(body))
	copy(s.fp.step2Data, body)
	s.mu.Unlock()

	sessionKey, err := deriveFPSessionKey(body, mode)
	if err != nil {
		log.Printf("[fp-setup] step 2: session key derivation failed: %v", err)
		// Continue anyway — return the echo response, log the failure
	} else {
		s.mu.Lock()
		s.fp.sessionKey = sessionKey
		s.mu.Unlock()
		log.Printf("[fp-setup] step 2: session key derived: %x", sessionKey)
	}

	resp := make([]byte, 32)
	copy(resp[:12], fpRespHeader[:])
	copy(resp[12:], body[144:164])
	log.Printf("[fp-setup] step 2: mode=%d resp=%x", mode, resp)
	return resp, true
}

// deriveFPSessionKey derives the 16-byte FairPlay session key from the step-2 request.
// It AES-128-ECB-decrypts bytes [12:28] of the request using the mode-specific master key.
func deriveFPSessionKey(step2 []byte, mode int) ([]byte, error) {
	if len(step2) < 28 {
		return nil, fmt.Errorf("step2 too short: %d", len(step2))
	}
	if mode > 3 {
		return nil, fmt.Errorf("invalid mode: %d", mode)
	}
	masterKey := fpMasterKeys[mode][:]
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, fmt.Errorf("AES cipher: %w", err)
	}
	// AES-128-ECB decrypt bytes [12:28] → session key
	sessionKey := make([]byte, 16)
	block.Decrypt(sessionKey, step2[12:28])
	return sessionKey, nil
}

// decryptFPAESKey extracts and decrypts the audio AES key from an fpaeskey blob.
// The blob structure (from UxPlay fair_play.cpp):
//
//	[0-3]   FPLY magic
//	[4]     version (1)
//	[5]     type (2)
//	[6]     mode
//	[7-35]  header data
//	[32-35] uint32be length of encrypted key (should be 16)
//	[36-51] encrypted audio AES key (AES-128-ECB with session key)
func decryptFPAESKey(blob, sessionKey []byte) ([]byte, error) {
	if len(blob) < 52 {
		return nil, fmt.Errorf("fpaeskey blob too short: %d bytes", len(blob))
	}
	if string(blob[0:4]) != "FPLY" {
		return nil, fmt.Errorf("fpaeskey bad magic: %x", blob[0:4])
	}
	keyLen := int(binary.BigEndian.Uint32(blob[32:36]))
	log.Printf(
		"[fp] fpaeskey: version=%d type=%d mode=%d keyLen=%d",
		blob[4],
		blob[5],
		blob[6],
		keyLen,
	)
	if keyLen != 16 {
		return nil, fmt.Errorf("unexpected key length in fpaeskey: %d", keyLen)
	}
	encKey := blob[36:52]
	log.Printf("[fp] encrypted audio key: %x", encKey)

	block, err := aes.NewCipher(sessionKey)
	if err != nil {
		return nil, fmt.Errorf("AES cipher: %w", err)
	}
	audioKey := make([]byte, 16)
	block.Decrypt(audioKey, encKey)
	return audioKey, nil
}

// --- RTSP helpers ---

type rtspReq struct {
	method  string
	uri     string
	headers map[string]string
	body    []byte
}

type rtspResp struct {
	status  int
	reason  string
	headers map[string]string
	body    []byte
	close   bool
}

func parseRTSP(r *bufio.Reader, firstLine string) (*rtspReq, error) {
	parts := strings.SplitN(firstLine, " ", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("bad request line: %q", firstLine)
	}
	req := &rtspReq{method: parts[0], uri: parts[1], headers: make(map[string]string)}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		kv := strings.SplitN(line, ":", 2)
		if len(kv) == 2 {
			req.headers[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}
	if cl, ok := req.headers["Content-Length"]; ok {
		n, _ := strconv.Atoi(cl)
		if n > 0 {
			req.body = make([]byte, n)
			io.ReadFull(r, req.body) //nolint:errcheck
		}
	}
	return req, nil
}

func writeResp(w io.Writer, resp *rtspResp) error {
	if resp.headers == nil {
		resp.headers = make(map[string]string)
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

func parseSDP(body []byte) map[string]string {
	out := make(map[string]string)
	var lastKey string
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			lastKey = ""
			continue
		}
		if len(line) > 2 && line[1] == '=' {
			if strings.HasPrefix(line, "a=") {
				kv := strings.SplitN(line[2:], ":", 2)
				if len(kv) == 2 {
					lastKey = strings.TrimSpace(kv[0])
					out[lastKey] = strings.TrimSpace(kv[1])
				}
			} else {
				lastKey = ""
			}
		} else if lastKey != "" {
			out[lastKey] += line
		}
	}
	return out
}

func parseTransport(t string) (audio, ctrl, timing int) {
	for _, p := range strings.Split(t, ";") {
		p = strings.TrimSpace(p)
		switch {
		case strings.HasPrefix(p, "control_port="):
			ctrl, _ = strconv.Atoi(strings.TrimPrefix(p, "control_port="))
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

func padB64(s string) string {
	if r := len(s) % 4; r != 0 {
		s += strings.Repeat("=", 4-r)
	}
	return s
}

func formatMAC(hw string) string {
	if len(hw) != 12 {
		return hw
	}
	return fmt.Sprintf("%s:%s:%s:%s:%s:%s", hw[0:2], hw[2:4], hw[4:6], hw[6:8], hw[8:10], hw[10:12])
}

func bad(cseq string) *rtspResp {
	return &rtspResp{status: 400, reason: "Bad Request", headers: map[string]string{"CSeq": cseq}}
}

func internalErr(cseq string) *rtspResp {
	return &rtspResp{
		status:  500,
		reason:  "Internal Server Error",
		headers: map[string]string{"CSeq": cseq},
	}
}

func safeIdx(b []byte, i int) byte {
	if i < len(b) {
		return b[i]
	}
	return 0
}

func loadRSAKey() (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(airportExpressKey))
	if block == nil {
		return nil, fmt.Errorf("PEM decode failed")
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

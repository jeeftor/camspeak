package airplay

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"

	clog "github.com/charmbracelet/log"
	"github.com/grandcat/zeroconf"

	"github.com/jeeftor/camspeak/internal/logging"
)

// Server is a RAOP (AirPlay v1) receiver that listens for AirPlay connections
// and routes received audio to a camera speaker.
type Server struct {
	name        string // AirPlay device name (shown in iOS AirPlay picker)
	port        int    // RTSP listener port
	hwAddr      string // fake MAC address for mDNS registration
	advertiseIP string // IP to advertise in mDNS (empty = auto-detect all interfaces)
	model       string // device model string advertised in mDNS (controls iOS icon)
	rsaKey      *rsa.PrivateKey
	edPriv      ed25519.PrivateKey // Ed25519 key for AirPlay pairing
	pkHex       string             // Ed25519 public key in hex (for pk= TXT record)
	piUUID      string             // Pairing identity UUID (for pi= TXT record)
	speaker     Speaker
	log         *clog.Logger
	listener    net.Listener
	zeroconf    *zeroconf.Server // RAOP _raop._tcp
	airplayZC   *zeroconf.Server // AirPlay _airplay._tcp

	primeSilenceMs int     // ms of silence to write before first real audio
	gain           float64 // digital gain applied to audio before sending to camera

	// Active session
	sessionMu sync.Mutex
	session   *session

	// FairPlay per-connection state (mode derived in step 1, session key in step 2)
	fpMu         sync.Mutex
	fpMode       int
	fpSessionKey []byte
}

// SendTiming breaks down SendRaw into latency vs playback (mirrors cameras.SendTiming).
type SendTiming struct {
	OpenMs     int64
	PlaybackMs int64
}

// Speaker is the interface for sending raw G.711ulaw audio to a camera.
// This matches cameras.Speaker but we define it locally to avoid import cycles.
type Speaker interface {
	SendRaw(rawFile string) (SendTiming, error)
	Stream(r io.Reader) error
	Stop() error
}

// NewServer creates a RAOP receiver for the given camera name.
// The name appears in the iOS AirPlay picker.
// advertiseIP is the IP address to advertise in mDNS (important for Docker host
// networking where bridge interfaces shouldn't be advertised). If empty, all
// interfaces are used.
func NewServer(
	name string,
	port int,
	advertiseIP string,
	speaker Speaker,
	model string,
	gain float64,
) (*Server, error) {
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
		model:       model,
		rsaKey:      key,
		edPriv:      edPriv,
		pkHex:       pkHex,
		piUUID:      piUUID,
		speaker:     speaker,
		log:         logging.New("airplay", clog.InfoLevel).With("camera", name),
		gain:        gain,
	}, nil
}

// SetLogLevel changes the log level for this AirPlay server.
// Pass clog.DebugLevel for verbose protocol logging.
func (s *Server) SetLogLevel(level clog.Level) {
	logging.SetLevel(s.log, level)
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
		"et=0",     // no encryption — avoid FairPlay /fp-setup
		"md=0,1,2", // text, artwork, progress
		"pw=false",
		"sv=false",
		"sr=44100",
		"ss=16",
		"tp=UDP",
		"vn=65537",
		"vs=366.0",
		"am=" + s.model,
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
		"model=" + s.model,
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
	s.log.Info("AirPlay: client connected", "from", remote)

	reader := bufio.NewReader(conn)
	for {
		// Read the first line to determine if this is RTSP or HTTP
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				s.log.Debug("read error", "err", err, "from", remote)
			}
			s.log.Info("AirPlay: client disconnected", "from", remote)
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

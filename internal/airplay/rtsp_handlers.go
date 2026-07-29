package airplay

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// handleAnnounce parses the SDP, extracts AES key/IV, handles RSA challenge,
// and creates a new session.
func (s *Server) handleAnnounce(req *rtspRequest, cseq string) *rtspResponse {
	sdp := parseSDP(req.body)
	s.log.Debug("ANNOUNCE SDP", "sdp", redactSDP(req.body))

	// Handle Apple-Challenge (RSA authentication) — may appear in ANNOUNCE too
	var appleResponse string
	if resp, ok := s.handleAppleChallenge(req); ok {
		appleResponse = resp
	}

	// Extract AES key — iOS 18 uses fpaeskey (FairPlay), older iOS uses rsaaeskey (RSA)
	var aesKey []byte
	if fpKeyB64, ok := sdp["fpaeskey"]; ok {
		// FairPlay path: audio AES key is encrypted with the FP session key from /fp-setup
		s.log.Debug("ANNOUNCE: FairPlay mode (fpaeskey)")
		fpKeyB64 = strings.Join(strings.Fields(fpKeyB64), "")
		fpBlob, err := base64.StdEncoding.DecodeString(padBase64(fpKeyB64))
		if err != nil {
			s.log.Warn("ANNOUNCE: bad fpaeskey base64", "err", err)
			return &rtspResponse{
				status:  400,
				reason:  "Bad Request",
				headers: map[string]string{"CSeq": cseq},
			}
		}
		s.fpMu.Lock()
		sessionKey := s.fpSessionKey
		s.fpMu.Unlock()
		if sessionKey == nil {
			s.log.Warn("ANNOUNCE: fpaeskey present but no FP session key — fp-setup incomplete")
			return &rtspResponse{
				status:  400,
				reason:  "Bad Request",
				headers: map[string]string{"CSeq": cseq},
			}
		}
		aesKey, err = decryptFPAESKey(fpBlob, sessionKey)
		if err != nil {
			s.log.Warn("ANNOUNCE: fpaeskey decrypt failed", "err", err)
			return &rtspResponse{
				status:  400,
				reason:  "Bad Request",
				headers: map[string]string{"CSeq": cseq},
			}
		}
		s.log.Info("ANNOUNCE: FairPlay AES key decrypted",
			"blob_len", len(fpBlob),
		)
	} else if rsaAesKey, ok := sdp["rsaaeskey"]; ok {
		// Legacy RSA path
		s.log.Debug("ANNOUNCE: RSA mode (rsaaeskey)")
		rsaAesKey = strings.Join(strings.Fields(rsaAesKey), "")
		encryptedAesKey, err := base64.StdEncoding.DecodeString(padBase64(rsaAesKey))
		if err != nil {
			s.log.Warn("ANNOUNCE: bad rsaaeskey base64", "err", err)
			return &rtspResponse{status: 400, reason: "Bad Request", headers: map[string]string{"CSeq": cseq}}
		}
		aesKey, err = rsa.DecryptOAEP(sha1.New(), rand.Reader, s.rsaKey, encryptedAesKey, nil)
		if err != nil {
			s.log.Warn("ANNOUNCE: RSA decrypt failed", "err", err)
			return &rtspResponse{status: 400, reason: "Bad Request", headers: map[string]string{"CSeq": cseq}}
		}
	} else {
		s.log.Warn("ANNOUNCE: no fpaeskey or rsaaeskey in SDP")
		return &rtspResponse{status: 400, reason: "Bad Request", headers: map[string]string{"CSeq": cseq}}
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
		aesKey:         aesKey,
		aesIV:          aesIV,
		fmtp:           fmtp,
		log:            s.log,
		speaker:        s.speaker,
		primeSilenceMs: s.primeSilenceMs,
		gain:           s.gain,
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

	s.log.Info("AirPlay: RECORD — client started audio playback")

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

// redactSDP returns a sanitized copy of an ANNOUNCE SDP body with key
// material (fpaeskey, rsaaeskey, aesiv) replaced so it can be logged safely.
func redactSDP(body []byte) string {
	var out strings.Builder
	var lastKey string
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			lastKey = ""
			out.WriteString(line + "\n")
			continue
		}

		if len(trimmed) > 2 && trimmed[1] == '=' {
			if strings.HasPrefix(trimmed, "a=") {
				kv := strings.SplitN(trimmed[2:], ":", 2)
				if len(kv) == 2 {
					lastKey = strings.TrimSpace(kv[0])
					if lastKey == "fpaeskey" || lastKey == "rsaaeskey" || lastKey == "aesiv" {
						out.WriteString("a=" + lastKey + ":[redacted]\n")
						continue
					}
				} else {
					lastKey = ""
				}
			} else {
				lastKey = ""
			}
		} else if lastKey != "" &&
			(lastKey == "fpaeskey" || lastKey == "rsaaeskey" || lastKey == "aesiv") {
			// Drop continuation lines for redacted keys.
			continue
		}
		out.WriteString(line + "\n")
	}
	return strings.TrimRight(out.String(), "\n")
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

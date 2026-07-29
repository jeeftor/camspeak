package airplay

import (
	"bufio"
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"strings"
)

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
				"CSeq":              cseq,
				"Public":            "ANNOUNCE, SETUP, RECORD, PAUSE, FLUSH, TEARDOWN, OPTIONS, GET_PARAMETER, SET_PARAMETER, POST",
				"Audio-Jack-Status": "connected; type=analog",
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
		var respBody string
		if strings.Contains(string(req.body), "volume") {
			respBody = "volume: -20.000000\r\n"
		}
		hdrs := map[string]string{"CSeq": cseq}
		if respBody != "" {
			hdrs["Content-Type"] = "text/parameters"
		}
		return &rtspResponse{status: 200, reason: "OK", headers: hdrs, body: []byte(respBody)}

	case "POST":
		// FairPlay setup — iOS sends 16 bytes (step 1) or 164 bytes (step 2)
		if strings.HasPrefix(req.uri, "/fp-setup") {
			if len(req.body) <= 16 {
				// Step 1: return 142-byte FairPlay certificate; save mode for step 2
				resp, ok := fairplaySetup(req.body)
				if !ok {
					s.log.Debug("fp-setup step 1 failed", "body_len", len(req.body))
					return &rtspResponse{
						status:  400,
						reason:  "Bad Request",
						headers: map[string]string{"CSeq": cseq},
					}
				}
				mode := int(req.body[14])
				s.fpMu.Lock()
				s.fpMode = mode
				s.fpSessionKey = nil
				s.fpMu.Unlock()
				s.log.Debug("fp-setup step 1", "mode", mode, "resp_len", len(resp))
				return &rtspResponse{
					status: 200, reason: "OK",
					headers: map[string]string{
						"CSeq":         cseq,
						"Content-Type": "application/octet-stream",
					},
					body: resp,
				}
			}
			// Step 2: derive session key, return 32-byte handshake response
			resp, ok := fairplayHandshake(req.body)
			if !ok {
				s.log.Debug("fp-setup step 2 failed", "body_len", len(req.body))
				return &rtspResponse{
					status:  400,
					reason:  "Bad Request",
					headers: map[string]string{"CSeq": cseq},
				}
			}
			// Mode for key derivation comes from step2 body byte[6], NOT from step1.
			// RPiPlay/goplay2 both read mode from the step2 request directly.
			mode := int(req.body[6])
			sessionKey, err := deriveFPSessionKey(req.body, mode)
			if err != nil {
				s.log.Warn("fp-setup step 2: session key derivation failed", "err", err)
			} else {
				s.fpMu.Lock()
				s.fpSessionKey = sessionKey
				s.fpMu.Unlock()
				s.log.Info("fp-setup step 2: session key derived",
					"mode", mode,
					"step2_prefix", fmt.Sprintf("%x", req.body[:min(32, len(req.body))]),
					"session_key", fmt.Sprintf("%x", sessionKey),
				)
			}
			return &rtspResponse{
				status: 200, reason: "OK",
				headers: map[string]string{
					"CSeq":         cseq,
					"Content-Type": "application/octet-stream",
				},
				body: resp,
			}
		}
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

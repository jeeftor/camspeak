package airplay

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"net"
	"time"

	clog "github.com/charmbracelet/log"
)

// session holds the state of a single RAOP connection.
type session struct {
	aesKey         []byte
	aesIV          []byte
	fmtp           string
	log            *clog.Logger
	speaker        Speaker
	primeSilenceMs int
	gain           float64

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
	stream, err := newAudioStream(s.speaker, s.log, s.primeSilenceMs, s.gain)
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

	pktCount := 0
	decodeCount := 0
	var bytesReceived int64
	var bytesDecoded int64
	lastSummary := time.Now()

	for {
		select {
		case <-s.done:
			s.log.Info("audio: stream ended", "packets", pktCount, "decoded", decodeCount,
				"bytes_received", bytesReceived, "bytes_decoded", bytesDecoded)
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

		pktCount++
		bytesReceived += int64(len(payload))
		seqNum := int(buf[2])<<8 | int(buf[3])
		if pktCount == 1 {
			s.log.Info(
				"audio: first RTP packet received",
				"seq",
				seqNum,
				"payloadLen",
				len(payload),
			)
		}
		s.log.Debug("audio: RTP packet",
			"seq", seqNum, "payloadLen", len(payload), "totalLen", n)

		// Decrypt with AES-128-CBC.
		// RAOP only encrypts the 16-byte-aligned prefix; the tail is plaintext.
		decrypted := make([]byte, len(payload))
		alignedLen := len(payload) &^ 0xf // round down to multiple of 16
		if alignedLen > 0 {
			iv := make([]byte, 16)
			copy(iv, s.aesIV)
			cipher.NewCBCDecrypter(block, iv).
				CryptBlocks(decrypted[:alignedLen], payload[:alignedLen])
		}
		copy(decrypted[alignedLen:], payload[alignedLen:]) // unencrypted tail

		// Decode ALAC frame → PCM 16-bit stereo 44100Hz.
		// Small frames (<64 bytes) are silence/sync packets — skip them.
		// Also recover from panics in the ALAC library for malformed frames.
		if len(decrypted) < 64 {
			s.log.Debug("audio: skipping small frame", "len", len(decrypted))
			continue
		}
		pcm := alacDecodeSafe(s.decoder, decrypted)
		if len(pcm) == 0 {
			// Fallback: try the raw (undecrypted) payload — if this works,
			// the stream is unencrypted despite fpaeskey being present.
			rawPCM := alacDecodeSafe(s.decoder, payload)
			if len(rawPCM) > 0 {
				s.log.Info("audio: raw payload decoded — stream is UNENCRYPTED",
					"seq", seqNum, "payloadLen", len(payload))
				pcm = rawPCM
			} else {
				s.log.Debug("audio: ALAC decode returned empty",
					"encryptedLen", len(payload),
					"raw0", fmt.Sprintf("%02x", payload[0]),
					"dec0", fmt.Sprintf("%02x", decrypted[0]),
				)
				continue
			}
		}

		decodeCount++
		bytesDecoded += int64(len(pcm))
		s.log.Debug("audio: decoded ALAC", "pcmLen", len(pcm), "seq", seqNum)

		// Feed PCM to the audio stream (which pipes to ffmpeg → G.711ulaw → camera)
		s.stream.writePCM(pcm)

		if time.Since(lastSummary) >= 5*time.Second {
			s.log.Info("audio: RTP summary",
				"packets", pktCount,
				"bytes_received", bytesReceived,
				"decoded", decodeCount,
				"bytes_decoded", bytesDecoded,
			)
			lastSummary = time.Now()
		}
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

// flush is a no-op in streaming mode — the session stays alive until teardown.
func (s *session) flush() {}

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

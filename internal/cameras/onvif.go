package cameras

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v4"
	"github.com/bluenviron/gortsplib/v4/pkg/base"
	"github.com/bluenviron/gortsplib/v4/pkg/description"
	"github.com/bluenviron/gortsplib/v4/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/g711"
	clog "github.com/charmbracelet/log"
)

// OnvifClient plays audio on a camera via ONVIF RTSP backchannel.
// It connects directly to the camera's RTSP server, negotiates the
// backchannel (a=sendonly audio track), and sends G.711 RTP packets.
// No external dependency (go2rtc/ffmpeg) required.
type OnvifClient struct {
	rtspURL string // e.g. "rtsp://user:pass@192.168.1.195:554/stream0"
	ip      string // camera IP (for ping)
	log     *clog.Logger

	// Active stream tracking for Stop()
	activeMu  sync.Mutex
	activeCli *gortsplib.Client // active RTSP client
	stopped   bool              // set by Stop() to suppress write errors
}

// NewOnvifClient creates a client that uses ONVIF RTSP backchannel.
// rtspURL is the full RTSP URL including credentials.
func NewOnvifClient(rtspURL, ip string) *OnvifClient {
	return &OnvifClient{
		rtspURL: rtspURL,
		ip:      ip,
		log:     newLogger("onvif"),
	}
}

// findG711BackChannel searches the SDP for a sendonly G.711 audio track.
func findG711BackChannel(desc *description.Session) (*description.Media, *format.G711) {
	for _, media := range desc.Medias {
		if media.IsBackChannel {
			for _, forma := range media.Formats {
				if g, ok := forma.(*format.G711); ok {
					return media, g
				}
			}
		}
	}
	return nil, nil
}

// SendRaw plays a raw G.711ulaw 8kHz file on the camera via RTSP backchannel.
// It reads the raw file, converts G.711ulaw → LPCM, encodes to RTP, and
// sends packets at real-time speed (8000 samples/sec).
func (c *OnvifClient) SendRaw(rawFile string) error {
	// Reset stopped flag from any previous Stop() call
	c.activeMu.Lock()
	c.stopped = false
	c.activeMu.Unlock()

	// Read the raw G.711ulaw file
	rawData, err := os.ReadFile(rawFile)
	if err != nil {
		return fmt.Errorf("reading raw file: %w", err)
	}

	if len(rawData) == 0 {
		return fmt.Errorf("raw file is empty")
	}

	// Parse the RTSP URL
	u, err := base.ParseURL(c.rtspURL)
	if err != nil {
		return fmt.Errorf("parsing RTSP URL: %w", err)
	}

	// Create RTSP client with backchannel support
	client := gortsplib.Client{
		Scheme:              u.Scheme,
		Host:                u.Host,
		RequestBackChannels: true,
	}

	if err := client.Start2(); err != nil {
		return fmt.Errorf("connecting to RTSP server: %w", err)
	}

	// Track active client for Stop()
	c.activeMu.Lock()
	c.activeCli = &client
	c.activeMu.Unlock()

	defer func() {
		client.Close()
		c.activeMu.Lock()
		c.activeCli = nil
		c.activeMu.Unlock()
	}()

	// Describe to get the SDP (with backchannel tracks)
	desc, _, err := client.Describe(u)
	if err != nil {
		return fmt.Errorf("RTSP DESCRIBE: %w", err)
	}

	// Find the G.711 backchannel
	medi, forma := findG711BackChannel(desc)
	if medi == nil {
		return fmt.Errorf(
			"no G.711 backchannel found in RTSP SDP — camera may not support two-way audio",
		)
	}

	c.log.Info("found backchannel",
		"mulaw", forma.MULaw,
		"sample_rate", forma.ClockRate(),
		"channels", forma.ChannelCount,
	)

	// Setup the backchannel media
	if _, err := client.Setup(desc.BaseURL, medi, 0, 0); err != nil {
		return fmt.Errorf("RTSP SETUP backchannel: %w", err)
	}

	// Start PLAY
	if _, err := client.Play(nil); err != nil {
		return fmt.Errorf("RTSP PLAY: %w", err)
	}

	// Create RTP encoder for G.711
	rtpEnc, err := forma.CreateEncoder()
	if err != nil {
		return fmt.Errorf("creating RTP encoder: %w", err)
	}

	// Generate random timestamp start
	randomStart, err := randUint32()
	if err != nil {
		return fmt.Errorf("generating random start: %w", err)
	}

	// Convert G.711ulaw raw bytes → LPCM samples (16-bit, big-endian)
	// The g711.Mulaw.Unmarshal expects []byte of LPCM and produces G.711,
	// but we have G.711 and need LPCM. We need to decode mulaw → LPCM first.
	lpcmSamples := decodeMulaw(rawData)

	// Send in 100ms chunks (800 samples per chunk at 8kHz)
	const chunkSize = 800 // 100ms at 8kHz
	const tickerInterval = 100 * time.Millisecond

	ticker := time.NewTicker(tickerInterval)
	defer ticker.Stop()

	totalSamples := len(lpcmSamples) / 2 // 16-bit = 2 bytes per sample
	sentSamples := 0

	for sentSamples < totalSamples {
		<-ticker.C

		remaining := totalSamples - sentSamples
		n := chunkSize
		if remaining < chunkSize {
			n = remaining
		}

		// Extract n samples (n*2 bytes) from the LPCM buffer
		start := sentSamples * 2
		end := start + n*2
		chunk := lpcmSamples[start:end]

		// Current PTS
		pts := int64(sentSamples)

		// Encode LPCM → G.711
		var g711Samples []byte
		if forma.MULaw {
			g711Samples, err = g711.Mulaw(chunk).Marshal()
		} else {
			g711Samples, err = g711.Alaw(chunk).Marshal()
		}
		if err != nil {
			return fmt.Errorf("encoding G.711: %w", err)
		}

		// Generate RTP packets
		pkts, err := rtpEnc.Encode(g711Samples)
		if err != nil {
			return fmt.Errorf("encoding RTP: %w", err)
		}

		// Write RTP packets
		for _, pkt := range pkts {
			pkt.Timestamp = uint32(int64(randomStart) + pts)
			if err := client.WritePacketRTP(medi, pkt); err != nil {
				// Check if Stop() was called — intentional cancellation
				c.activeMu.Lock()
				wasStopped := c.stopped
				c.activeMu.Unlock()
				if wasStopped {
					c.log.Debug("send: stopped by user", "samples", sentSamples, "total", totalSamples)
					return nil
				}
				return fmt.Errorf("writing RTP packet: %w", err)
			}
		}

		sentSamples += n
	}

	c.log.Info("audio sent", "samples", sentSamples, "duration_ms", sentSamples/8)

	return nil
}

// Stream is not yet implemented for ONVIF; it buffers r and calls SendRaw.
func (c *OnvifClient) Stream(r io.Reader) error {
	tmp, err := os.CreateTemp("", "camspeak-onvif-*.raw")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	return c.SendRaw(tmp.Name())
}

// Stop immediately stops audio playback by closing the active RTSP client.
func (c *OnvifClient) Stop() error {
	c.activeMu.Lock()
	cli := c.activeCli
	c.stopped = true // suppress write errors in the streaming loop
	c.activeMu.Unlock()

	if cli == nil {
		return nil // nothing playing
	}

	c.log.Info("stop: stopping audio", "ip", c.ip)
	cli.Close()

	c.activeMu.Lock()
	c.activeCli = nil
	c.activeMu.Unlock()

	return nil
}

// Ping checks if the camera is reachable via TCP on port 80 or 554.
func (c *OnvifClient) Ping() bool {
	if tcpPing(c.ip, 80, 5*time.Second) {
		return true
	}
	return tcpPing(c.ip, 554, 5*time.Second)
}

// decodeMulaw converts G.711 mu-law bytes to 16-bit big-endian LPCM.
// The g711 package expects big-endian LPCM input for Mulaw.Marshal.
func decodeMulaw(mulawData []byte) []byte {
	// G.711 mu-law to linear PCM conversion table
	// Using the standard ITU G.711 mu-law decoding
	lpcm := make([]byte, len(mulawData)*2)

	for i, b := range mulawData {
		sample := mulawToLinear(b)
		lpcm[i*2] = byte(sample >> 8)
		lpcm[i*2+1] = byte(sample)
	}

	return lpcm
}

// mulawToLinear converts a single mu-law byte to a 16-bit linear PCM value.
func mulawToLinear(u byte) int16 {
	u = ^u
	sign := (u & 0x80) >> 7
	exponent := (u & 0x70) >> 4
	mantissa := u & 0x0F

	sample := int16(((mantissa << 3) + 0x84) << uint(exponent))
	sample -= 0x84

	if sign == 1 {
		sample = -sample
	}

	return sample
}

// randUint32 generates a random uint32 for RTP timestamp offset.
func randUint32() (uint32, error) {
	var b [4]byte
	_, err := rand.Read(b[:])
	if err != nil {
		return 0, err
	}
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3]), nil
}

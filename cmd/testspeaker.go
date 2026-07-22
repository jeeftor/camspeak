package cmd

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	digest "github.com/icholy/digest"
	"github.com/spf13/cobra"
)

// voiceSegment is one `say` utterance in the test sequence.
type voiceSegment struct {
	voice string
	text  string
}

var voiceSegments = []voiceSegment{
	{"Zarvox", "Hello my friend. I am Zarvox. Can you hear me through this camera speaker?"},
	{"Cellos", "I am cellos. I am singing to you. La la la la la."},
	{"Boing", "One. Two. Three. Four. Five. Boing boing boing."},
	{"Fred", "This is Fred. The audio streaming system appears to be working correctly."},
	{"Whisper", "I am whispering now. Can you still hear me clearly?"},
	{"Bad News", "Bad news. This is a test. Bad news bad news bad news."},
	{"Good News", "Good news! The camera speaker streaming system is working perfectly!"},
	{"Zarvox", "End of test. Goodbye from Zarvox."},
}

var testSpeakerCmd = &cobra.Command{
	Use:   "test-speaker",
	Short: "Stream multi-voice speech to a Hikvision camera speaker",
	Long: `Uses macOS 'say' or Linux 'espeak/espeak-ng' to synthesize speech in
multiple voices, then streams the audio as a single continuous session to verify
the camera speaker works. No gaps between segments — one open/stream/close for
the full sequence.`,
	RunE: runTestSpeaker,
}

// platformAudioExt returns the intermediate audio file extension for this OS.
func platformAudioExt() string {
	if runtime.GOOS == "darwin" {
		return ".aiff"
	}
	return ".wav"
}

// platformAudioFormat returns the ffmpeg muxer name for the intermediate files.
func platformAudioFormat() string {
	if runtime.GOOS == "darwin" {
		return "aiff"
	}
	return "wav"
}

// platformAudioCodec returns the ffmpeg audio codec for the intermediate files.
func platformAudioCodec() string {
	if runtime.GOOS == "darwin" {
		return "pcm_s16be"
	}
	return "pcm_s16le"
}

// synthCommand returns an OS-specific exec.Cmd to synthesize a voice segment.
// macOS uses `say`; Linux uses `espeak-ng` if available, otherwise `espeak`.
func synthCommand(seg voiceSegment, output string) *exec.Cmd {
	if runtime.GOOS == "darwin" {
		return exec.Command("say", "-v", seg.voice, "-r", "140", seg.text, "-o", output)
	}
	if _, err := exec.LookPath("espeak-ng"); err == nil {
		return exec.Command("espeak-ng", "-s", "140", "-v", "en", "-w", output, seg.text)
	}
	return exec.Command("espeak", "-s", "140", "-v", "en", "-w", output, seg.text)
}

// silenceCmd returns an ffmpeg command that generates a silent intermediate audio file.
func silenceCmd(output string, seconds float64) *exec.Cmd {
	return exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("aevalsrc=0:c=mono:s=22050:d=%.1f", seconds),
		"-c:a", platformAudioCodec(), "-f", platformAudioFormat(), output)
}

var (
	tsCameraIP   string
	tsCameraUser string
	tsCameraPass string
	tsCameraPort int
)

func init() {
	rootCmd.AddCommand(testSpeakerCmd)
	testSpeakerCmd.Flags().StringVar(&tsCameraIP, "ip", "", "Camera IP address (required)")
	testSpeakerCmd.Flags().StringVar(&tsCameraUser, "user", "admin", "Camera username")
	testSpeakerCmd.Flags().StringVar(&tsCameraPass, "pass", "", "Camera password (required)")
	testSpeakerCmd.Flags().IntVar(&tsCameraPort, "channel", 1, "ISAPI two-way audio channel number")
	_ = testSpeakerCmd.MarkFlagRequired("ip")
	_ = testSpeakerCmd.MarkFlagRequired("pass")
}

var (
	tsStyleOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	tsStyleErr   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	tsStyleDim   = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	tsStyleBold  = lipgloss.NewStyle().Bold(true)
	tsStyleVoice = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
)

func runTestSpeaker(cmd *cobra.Command, _ []string) error {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return fmt.Errorf("test-speaker is only supported on macOS and Linux")
	}
	fmt.Println()
	fmt.Println(tsStyleBold.Render("  camspeak speaker test — multi-voice"))
	fmt.Printf("  Camera: %s  Channel: %d\n\n", tsCameraIP, tsCameraPort)

	tmpDir, err := os.MkdirTemp("", "camspeak-speakertest-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	// --- Step 1: Synthesize each voice segment ---
	ext := platformAudioExt()
	fmt.Println("  Synthesizing voice segments...")
	audioFiles := make([]string, 0, len(voiceSegments))
	for i, seg := range voiceSegments {
		audio := filepath.Join(tmpDir, fmt.Sprintf("%02d-%s%s", i, seg.voice, ext))
		fmt.Printf("  %s %-10s  %s\n",
			tsStyleDim.Render(fmt.Sprintf("[%d/%d]", i+1, len(voiceSegments))),
			tsStyleVoice.Render(seg.voice),
			tsStyleDim.Render(seg.text))
		synth := synthCommand(seg, audio)
		if out, err := synth.CombinedOutput(); err != nil {
			return fmt.Errorf("synthesis failed for voice %q: %w\n%s", seg.voice, err, out)
		}
		// Insert 0.4s silence between segments
		if i > 0 {
			silenceFile := filepath.Join(tmpDir, fmt.Sprintf("%02d-silence%s", i, ext))
			silence := silenceCmd(silenceFile, 0.4)
			silence.Stderr = nil
			if err := silence.Run(); err != nil {
				return fmt.Errorf("silence gen failed: %w", err)
			}
			audioFiles = append(audioFiles, silenceFile)
		}
		audioFiles = append(audioFiles, audio)
	}
	fmt.Println()

	// --- Step 2: Build ffmpeg concat → mulaw stream ---
	// Prime buffer: 0.5s silence at the very start so the camera's audio
	// engine is warmed up before the first word.
	primeFile := filepath.Join(tmpDir, "00-prime"+ext)
	prime := silenceCmd(primeFile, 0.5)
	prime.Stderr = nil
	if err := prime.Run(); err != nil {
		return fmt.Errorf("prime silence failed: %w", err)
	}
	allFiles := append([]string{primeFile}, audioFiles...)

	// Build filter_complex concat
	var inputs []string
	var filterParts []string
	for i, f := range allFiles {
		inputs = append(inputs, "-i", f)
		filterParts = append(filterParts, fmt.Sprintf("[%d:a]", i))
	}
	filterStr := strings.Join(filterParts, "") +
		fmt.Sprintf("concat=n=%d:v=0:a=1[out]", len(allFiles))

	ffBase := make([]string, 0, len(inputs)+10)
	ffBase = append(ffBase, inputs...)
	ffBase = append(ffBase,
		"-filter_complex", filterStr,
		"-map", "[out]",
		"-ar", "8000",
		"-ac", "1",
		"-c:a", "pcm_mulaw",
		"-f", "mulaw",
	)

	// Pre-encode to measure total size
	fmt.Print("  Encoding to G.711 mulaw... ")
	mulaw := filepath.Join(tmpDir, "output.raw")
	encArgs := make([]string, 0, len(ffBase)+2)
	encArgs = append(encArgs, ffBase...)
	encArgs = append(encArgs, mulaw, "-y")
	encCmd := exec.Command("ffmpeg", encArgs...)
	encCmd.Stderr = nil
	if err := encCmd.Run(); err != nil {
		// retry with stderr for diagnosis
		encCmd2 := exec.Command("ffmpeg", encArgs...)
		out, _ := encCmd2.CombinedOutput()
		return fmt.Errorf("ffmpeg encode failed: %w\n%s", err, out)
	}
	info, _ := os.Stat(mulaw)
	totalBytes := info.Size()
	durationSec := float64(totalBytes) / 8000
	fmt.Printf("%s  %s  (%.1fs, %d bytes)\n",
		tsStyleOK.Render("OK"),
		tsStyleDim.Render(mulaw),
		durationSec, totalBytes)
	fmt.Println()

	// --- Step 3: Open Hikvision session ---
	fmt.Print("  [1/4] Opening ISAPI channel... ")
	baseURL := fmt.Sprintf("http://%s/ISAPI/System/TwoWayAudio/channels/%d", tsCameraIP, tsCameraPort)
	httpClient := &http.Client{
		Transport: &digest.Transport{Username: tsCameraUser, Password: tsCameraPass},
		Timeout:   10 * time.Second,
	}
	// Clear stale session
	if req, _ := http.NewRequest(http.MethodPut, baseURL+"/close", nil); req != nil {
		if resp, err := httpClient.Do(req); err == nil {
			resp.Body.Close()
		}
	}
	req, _ := http.NewRequest(http.MethodPut, baseURL+"/open", nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		fmt.Println(tsStyleErr.Render("FAILED"))
		return fmt.Errorf("open channel: %w", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	sessionID := tsExtractXML(string(body), "sessionId")
	if sessionID == "" {
		fmt.Println(tsStyleErr.Render("FAILED"))
		return fmt.Errorf("no sessionId (HTTP %d): %s", resp.StatusCode, body)
	}
	fmt.Println(tsStyleOK.Render("OK") + tsStyleDim.Render("  session="+sessionID))
	defer func() {
		if req, _ := http.NewRequest(http.MethodPut,
			baseURL+"/close?sessionId="+sessionID, nil); req != nil {
			if r, err := httpClient.Do(req); err == nil {
				r.Body.Close()
			}
		}
		fmt.Println("  Session closed.")
	}()

	// --- Step 4: Digest auth handshake ---
	fmt.Print("  [2/4] Digest auth handshake... ")
	path := fmt.Sprintf("/ISAPI/System/TwoWayAudio/channels/%d/audioData?sessionId=%s",
		tsCameraPort, sessionID)
	authHeader, err := tsDigestAuth(tsCameraIP, path, tsCameraUser, tsCameraPass)
	if err != nil {
		fmt.Println(tsStyleErr.Render("FAILED"))
		return fmt.Errorf("digest auth: %w", err)
	}
	fmt.Println(tsStyleOK.Render("OK"))

	// --- Step 5: Open streaming TCP connection ---
	fmt.Print("  [3/4] Opening streaming connection... ")
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(tsCameraIP, "80"), 5*time.Second)
	if err != nil {
		fmt.Println(tsStyleErr.Render("FAILED"))
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Duration(durationSec+15) * time.Second))

	headers := fmt.Sprintf("PUT %s HTTP/1.1\r\n", path)
	headers += fmt.Sprintf("Host: %s\r\n", tsCameraIP)
	headers += "Content-Type: application/octet-stream\r\n"
	headers += fmt.Sprintf("Content-Length: %d\r\n", totalBytes)
	if authHeader != "" {
		headers += fmt.Sprintf("Authorization: %s\r\n", authHeader)
	}
	headers += "Connection: close\r\n\r\n"
	if _, err := conn.Write([]byte(headers)); err != nil {
		fmt.Println(tsStyleErr.Render("FAILED"))
		return fmt.Errorf("write headers: %w", err)
	}
	fmt.Println(tsStyleOK.Render("OK"))

	// --- Step 6: Stream at 8000 bytes/sec ---
	fmt.Printf("  [4/4] Streaming %.1fs of audio...\n\n", durationSec)
	f, err := os.Open(mulaw)
	if err != nil {
		return err
	}
	defer f.Close()

	var mu sync.Mutex
	stopped := false
	if err := tsStreamAt8k(conn, f, totalBytes, &stopped, &mu); err != nil {
		fmt.Printf("\n  %s stream error: %v\n", tsStyleErr.Render("WARN"), err)
	}

	// Read camera response
	respBuf := make([]byte, 512)
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, _ := conn.Read(respBuf)
	statusLine, _, _ := strings.Cut(string(respBuf[:n]), "\r\n")
	fmt.Printf("\n  %s  Camera: %s\n\n", tsStyleOK.Render("Done"), tsStyleDim.Render(statusLine))
	return nil
}

func tsStreamAt8k(w io.Writer, r io.Reader, total int64, stopped *bool, mu *sync.Mutex) error {
	const chunkSize = 800
	const interval = 100 * time.Millisecond

	buf := make([]byte, chunkSize)
	written := int64(0)
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
			written += int64(n)
			pct := float64(written) / float64(total) * 100
			fmt.Printf("\r  %s  %.0f%%  %d/%d bytes",
				tsProgressBar(pct, 30), pct, written, total)
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

func tsDigestAuth(ip, path, user, pass string) (string, error) {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, "80"), 5*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	probe := fmt.Sprintf("PUT %s HTTP/1.1\r\nHost: %s\r\nContent-Length: 0\r\n\r\n", path, ip)
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
		Method: http.MethodPut, URI: path, Username: user, Password: pass,
	})
	if err != nil {
		return "", err
	}
	return cred.String(), nil
}

func tsExtractXML(s, tag string) string {
	open, close := "<"+tag+">", "</"+tag+">"
	i := strings.Index(s, open)
	if i < 0 {
		return ""
	}
	i += len(open)
	j := strings.Index(s[i:], close)
	if j < 0 {
		return ""
	}
	return s[i : i+j]
}

func tsProgressBar(pct float64, width int) string {
	filled := min(int(pct/100*float64(width)), width)
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
}

package api

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// sanitizeFilename strips path separators and wildcards from a user-provided filename.
func sanitizeFilename(name string) string {
	base := filepath.Base(name)

	base = strings.Map(func(r rune) rune {
		if r == '*' || r == '?' || r == '/' || r == '\\' || r == ':' {
			return '_'
		}

		return r
	}, base)

	if base == "" || base == "." || base == ".." {
		base = "upload"
	}

	return base
}

// redactURL returns a URL string with any embedded credentials removed.
// It only strips the userinfo portion (user:pass@host); the rest of the URL
// is preserved for diagnostics.
func redactURL(u *url.URL) string {
	if u == nil {
		return ""
	}

	redacted := *u
	redacted.User = nil

	return redacted.String()
}

// GenerateBeep creates a temporary 800Hz 2s G.711ulaw raw file via ffmpeg.
func GenerateBeep(tmpDir string) (string, error) {
	wav, err := os.CreateTemp(tmpDir, "camspeak_beep_*.wav")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}

	wavName := wav.Name()
	wav.Close()

	defer os.Remove(wavName)

	raw, err := os.CreateTemp(tmpDir, "camspeak_beep_*.raw")
	if err != nil {
		return "", err
	}

	rawName := raw.Name()
	raw.Close()

	// Generate sine wave → WAV
	cmd := exec.Command("ffmpeg", "-y",
		"-f", "lavfi", "-i", "sine=frequency=800:duration=2",
		"-ar", "16000", "-ac", "1", "-f", "wav", wavName)
	if out, err := cmd.CombinedOutput(); err != nil {
		os.Remove(rawName)

		return "", fmt.Errorf("ffmpeg sine: %w\n%s", err, out)
	}

	// Transcode to G.711ulaw 8kHz raw
	cmd = exec.Command("ffmpeg", "-y",
		"-i", wavName,
		"-ar", "8000", "-ac", "1",
		"-c:a", "pcm_mulaw", "-f", "mulaw", rawName)
	if out, err := cmd.CombinedOutput(); err != nil {
		os.Remove(rawName)

		return "", fmt.Errorf("ffmpeg transcode: %w\n%s", err, out)
	}

	return rawName, nil
}

// wavBytesToRaw writes WAV bytes to a temp file, transcodes to G.711ulaw raw, returns the raw path.
// gain controls the volume multiplier (1.0 = no boost). Caller must os.Remove the returned path.
func wavBytesToRaw(wavBytes []byte, tmpDir string, gain float64) (string, error) {
	wav, err := os.CreateTemp(tmpDir, "camspeak_tts_*.wav")
	if err != nil {
		return "", err
	}
	wavName := wav.Name()
	defer os.Remove(wavName)

	if _, err := wav.Write(wavBytes); err != nil {
		wav.Close()
		return "", err
	}
	wav.Close()

	raw, err := os.CreateTemp(tmpDir, "camspeak_tts_*.raw")
	if err != nil {
		return "", err
	}
	rawName := raw.Name()
	raw.Close()

	if err := transcodeFileToRawGain(wavName, rawName, gain); err != nil {
		os.Remove(rawName)
		return "", err
	}

	return rawName, nil
}

// rawToWAV converts a G.711ulaw raw file back to WAV for browser preview.
func rawToWAV(rawFile, tmpDir string) (string, error) {
	wav, err := os.CreateTemp(tmpDir, "camspeak_preview_*.wav")
	if err != nil {
		return "", err
	}

	wavName := wav.Name()
	wav.Close()

	cmd := exec.Command("ffmpeg", "-y",
		"-f", "mulaw", "-ar", "8000", "-ac", "1",
		"-i", rawFile,
		wavName)
	if out, err := cmd.CombinedOutput(); err != nil {
		os.Remove(wavName)

		return "", fmt.Errorf("ffmpeg raw→wav: %w\n%s", err, out)
	}

	return wavName, nil
}

// boostRawGain re-processes an existing G.711ulaw raw file with a volume gain.
// Returns the path to a new temp raw file. Caller must os.Remove it.
func boostRawGain(srcRaw, tmpDir string, gain float64) (string, error) {
	out, err := os.CreateTemp(tmpDir, "camspeak_gain_*.raw")
	if err != nil {
		return "", err
	}
	outName := out.Name()
	out.Close()

	af := fmt.Sprintf("volume=%.1f", gain)
	cmd := exec.Command("ffmpeg", "-y",
		"-f", "mulaw", "-ar", "8000", "-ac", "1",
		"-i", srcRaw,
		"-af", af,
		"-c:a", "pcm_mulaw", "-f", "mulaw",
		outName,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		os.Remove(outName)
		return "", fmt.Errorf("ffmpeg gain boost: %w\n%s", err, out)
	}

	return outName, nil
}

// transcodeFileToRawGain converts any audio file to G.711ulaw 8kHz raw with a given gain.
func transcodeFileToRawGain(src, dst string, gain float64) error {
	af := fmt.Sprintf("volume=%.1f", gain)
	cmd := exec.Command("ffmpeg", "-y",
		"-i", src,
		"-af", af,
		"-ar", "8000",
		"-ac", "1",
		"-c:a", "pcm_mulaw",
		"-f", "mulaw",
		dst,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg: %w\n%s", err, out)
	}

	return nil
}

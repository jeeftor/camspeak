package api

import (
	"fmt"
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

// GenerateBeep creates a temporary 800Hz 2s G.711ulaw raw file via ffmpeg.
func GenerateBeep() (string, error) {
	wav, err := os.CreateTemp("", "camspeak_beep_*.wav")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}

	wavName := wav.Name()
	wav.Close()

	defer os.Remove(wavName)

	raw, err := os.CreateTemp("", "camspeak_beep_*.raw")
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

// rawToWAV converts a G.711ulaw raw file back to WAV for browser preview.
func rawToWAV(rawFile string) (string, error) {
	wav, err := os.CreateTemp("", "camspeak_preview_*.wav")
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

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

// wavBytesToRawWithPrime writes WAV bytes to a temp file, transcodes to G.711ulaw raw,
// and prepends primeMs of µ-law silence. gain controls the volume multiplier (1.0 = no boost).
// Caller must os.Remove the returned path. primeMs <= 0 skips silence padding.
func wavBytesToRawWithPrime(wavBytes []byte, tmpDir string, gain float64, primeMs int) (string, error) {
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

	if err := transcodeFileToRawGainWithPrime(wavName, rawName, gain, primeMs); err != nil {
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

// transcodeFileToRawGainWithPrime converts any audio file to G.711ulaw 8kHz raw
// primeMs milliseconds of G.711 µ-law silence to the output file. This warms
// the camera's audio engine so the first real audio isn't clipped/garbled.
// primeMs <= 0 skips the silence padding.
func transcodeFileToRawGainWithPrime(src, dst string, gain float64, primeMs int) error {
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

	if primeMs > 0 {
		if err := prependSilence(dst, primeMs); err != nil {
			return fmt.Errorf("prepending silence: %w", err)
		}
	}

	return nil
}

// prependSilence inserts primeMs milliseconds of G.711 µ-law silence at the
// beginning of the file at path. In µ-law, the silence code is 0xFF.
// At 8000 bytes/sec, primeMs maps to (primeMs * 8) bytes.
// The file is rewritten in place using a temp file + rename.
func prependSilence(path string, primeMs int) error {
	silenceBytes := make([]byte, primeMs*8) // 8000 bytes/sec = 8 bytes/ms
	for i := range silenceBytes {
		silenceBytes[i] = 0xFF // µ-law silence
	}

	original, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "camspeak_prime_*.raw")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(silenceBytes); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(original); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmpName, path)
}

// prependSilenceToNewFile reads the raw file at src, prepends primeMs of
// µ-law silence, and writes the result to a new temp file. Returns the temp
// file path (caller must os.Remove). If primeMs <= 0, returns src unchanged.
func prependSilenceToNewFile(src string, tmpDir string, primeMs int) (string, error) {
	if primeMs <= 0 {
		return src, nil
	}

	original, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}

	tmp, err := os.CreateTemp(tmpDir, "camspeak_prime_*.raw")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()

	silenceBytes := make([]byte, primeMs*8)
	for i := range silenceBytes {
		silenceBytes[i] = 0xFF
	}

	if _, err := tmp.Write(silenceBytes); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	if _, err := tmp.Write(original); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", err
	}

	return tmpName, nil
}

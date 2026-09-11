package tts

import (
	"fmt"
	"mime"
	"strconv"
)

// validatePCMFormat checks the raw sample contract shared by tests and playback.
func validatePCMFormat(contentType string, rate, channels int) error {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("invalid PCM Content-Type %q", contentType)
	}
	switch mediaType {
	case "audio/l16":
		// Standard L16 defaults to big-endian. Lemonade explicitly declares LE.
		if params["endianness"] != "little-endian" {
			return fmt.Errorf("audio/l16 must explicitly declare endianness=little-endian")
		}
		if params["channels"] == "" && channels != 1 {
			return fmt.Errorf("audio/l16 defaults to mono but preset requests %d channels", channels)
		}
	case "audio/pcm", "audio/raw", "application/octet-stream":
		if value := params["endianness"]; value != "" && value != "little-endian" {
			return fmt.Errorf("unsupported PCM endianness %q", value)
		}
	default:
		return fmt.Errorf("unsupported raw PCM Content-Type %q", contentType)
	}
	for key, want := range map[string]int{"rate": rate, "channels": channels} {
		if value := params[key]; value != "" {
			got, err := strconv.Atoi(value)
			if err != nil || got != want {
				return fmt.Errorf("PCM %s=%q does not match configured %d", key, value, want)
			}
		}
	}
	return nil
}

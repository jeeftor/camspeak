package util

import (
	"math"
	"testing"
)

func TestMulawRoundTrip(t *testing.T) {
	// Test that decode→encode→decode is stable (nearest-neighbor encode
	// guarantees the re-decoded value is the closest possible).
	for i := 0; i < 256; i++ {
		decoded := MulawDecode(byte(i))
		encoded := MulawEncode(decoded)
		redecoded := MulawDecode(encoded)
		if redecoded != decoded {
			t.Errorf("byte %d: round-trip failed: %d → %d", i, decoded, redecoded)
		}
	}
}

func TestApplyGainUnity(t *testing.T) {
	original := []byte{0, 128, 255, 64, 192}
	modified := make([]byte, len(original))
	copy(modified, original)
	ApplyGainMulaw(modified, 1.0)
	for i := range original {
		if modified[i] != original[i] {
			t.Errorf("gain=1.0 changed byte %d: %d → %d", i, original[i], modified[i])
		}
	}
}

func TestApplyGainAmp(t *testing.T) {
	// A mid-range sample should get louder with gain > 1.
	buf := []byte{128} // 128 = 0 in µ-law (silence)
	ApplyGainMulaw(buf, 3.0)
	// Silence * 3 = silence, should still be 128 (or close).
	if buf[0] != 128 {
		t.Errorf("silence * 3.0 = %d, want 128", buf[0])
	}

	// A non-silence sample should change.
	buf = []byte{64} // some non-zero sample
	original := MulawDecode(64)
	ApplyGainMulaw(buf, 3.0)
	amplified := MulawDecode(buf[0])
	if math.Abs(float64(amplified)) <= math.Abs(float64(original)) {
		t.Errorf("gain=3.0 did not amplify: %d → %d", original, amplified)
	}
}

func TestApplyGainClamp(t *testing.T) {
	// Maximum amplitude sample with high gain should not wrap around.
	buf := []byte{0} // 0 = -32124 in µ-law (near max negative)
	ApplyGainMulaw(buf, 10.0)
	result := MulawDecode(buf[0])
	// Should be clamped to -32768, not wrapped.
	if result > -30000 {
		t.Errorf("expected clamped negative value, got %d", result)
	}
}

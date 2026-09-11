package tts

import "testing"

func TestPCMFormat(t *testing.T) {
	for _, tc := range []struct {
		header         string
		rate, channels int
		valid          bool
	}{
		{"audio/l16;rate=24000;endianness=little-endian", 24000, 1, true},
		{"audio/L16; rate=24000; channels=2; endianness=little-endian", 24000, 2, true},
		{"audio/l16;rate=24000", 24000, 1, false},
		{"audio/l16;rate=24000;endianness=big-endian", 24000, 1, false},
		{"audio/l16;rate=48000;endianness=little-endian", 24000, 1, false},
		{"audio/l16;rate=24000;endianness=little-endian", 24000, 2, false},
		{"audio/pcm", 24000, 1, true},
		{"audio/wav", 24000, 1, false},
	} {
		t.Run(tc.header, func(t *testing.T) {
			if err := validatePCMFormat(tc.header, tc.rate, tc.channels); (err == nil) != tc.valid {
				t.Fatalf("validation=%v, valid=%v", err, tc.valid)
			}
		})
	}
}

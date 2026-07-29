package util

import "testing"

func TestDefaultGain(t *testing.T) {
	const def = 3.0
	cases := []struct {
		name string
		gain float64
		want float64
	}{
		{"zero returns default", 0, def},
		{"negative returns default", -1, def},
		{"positive kept", 2.5, 2.5},
		{"three kept", 3.0, 3.0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DefaultGain(tc.gain, def)
			if got != tc.want {
				t.Errorf("DefaultGain(%f, %f) = %f, want %f", tc.gain, def, got, tc.want)
			}
		})
	}
}

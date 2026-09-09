package util

import "math"

// DefaultGain returns gain if it is positive, otherwise the provided default.
// This centralises the "if gain <= 0 { gain = default }" pattern used across
// audio handlers and the AirPlay manager.
func DefaultGain(gain, def float64) float64 {
	if gain <= 0 {
		return def
	}
	return gain
}

// ComputeLevel computes an RMS audio level from a buffer of µ-law bytes,
// returning a normalized value in [0, 1] suitable for a VU meter.
// Uses the MulawDecode lookup table from mulaw.go.
func ComputeLevel(buf []byte) float64 {
	if len(buf) == 0 {
		return 0
	}
	var sumSq float64
	for _, b := range buf {
		f := float64(MulawDecode(b))
		sumSq += f * f
	}
	rms := math.Sqrt(sumSq / float64(len(buf)))
	normalized := rms / 32256.0
	if normalized < 0 {
		normalized = 0
	}
	return math.Sqrt(normalized)
}

package util

// DefaultGain returns gain if it is positive, otherwise the provided default.
// This centralises the "if gain <= 0 { gain = default }" pattern used across
// audio handlers and the AirPlay manager.
func DefaultGain(gain, def float64) float64 {
	if gain <= 0 {
		return def
	}
	return gain
}

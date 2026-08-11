package api

import "time"

// StepTimings tracks per-step elapsed durations for a request.
// Use Add to record a step, then Ms to get a map of millisecond values
// for JSON responses.
type StepTimings struct {
	steps map[string]time.Duration
}

// NewStepTimings creates a StepTimings with the given capacity.
func NewStepTimings(n int) *StepTimings {
	return &StepTimings{steps: make(map[string]time.Duration, n)}
}

// Add records the elapsed time since start for the given step name.
func (t *StepTimings) Add(name string, start time.Time) {
	if t.steps == nil {
		t.steps = make(map[string]time.Duration)
	}
	t.steps[name] = time.Since(start)
}

// Ms returns a map of step names to elapsed milliseconds.
func (t *StepTimings) Ms() map[string]int64 {
	out := make(map[string]int64, len(t.steps))
	for k, v := range t.steps {
		out[k] = v.Milliseconds()
	}
	return out
}

// TotalMs returns the total elapsed time from start in milliseconds.
func TotalMs(start time.Time) int64 {
	return time.Since(start).Milliseconds()
}

// TTFS returns the "time to first sound" in milliseconds — the sum of all
// latency steps (everything before the camera starts playing audio).
// This excludes send_playback_ms (the throttled streaming duration).
func (t *StepTimings) TTFS() int64 {
	var sum int64
	for k, v := range t.steps {
		if k == "send_playback_ms" {
			continue
		}
		sum += v.Milliseconds()
	}
	return sum
}

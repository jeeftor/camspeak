package api

import (
	"sync"
	"time"
)

// PlaybackState describes what a camera is currently doing audio-wise.
// State is one of: "playing", "paused", "idle".
// Source is the action that started playback: "stream", "speak", "play",
// "play-url", "beep".
// Detail is a human-readable identifier — the stream URL, TTS text, preset
// name, etc.
// Level is the current audio level (0.0–1.0) for VU meter display, only
// meaningful for streams and looped presets (when a streamSession is active).
type PlaybackState struct {
	State     string     `json:"state"`
	Source    string     `json:"source,omitempty"`
	Detail    string     `json:"detail,omitempty"`
	StartedAt time.Time  `json:"started_at,omitempty"`
	PausedAt  *time.Time `json:"paused_at,omitempty"`
	Level     float64    `json:"level,omitempty"`
}

var (
	playbackStates   = make(map[string]*PlaybackState)
	playbackStatesMu sync.RWMutex
)

// setPlayback records that a camera has started or resumed playback.
func setPlayback(camera, source, detail string) {
	playbackStatesMu.Lock()
	playbackStates[camera] = &PlaybackState{
		State:     "playing",
		Source:    source,
		Detail:    detail,
		StartedAt: time.Now(),
	}
	playbackStatesMu.Unlock()
}

// setPlaybackPaused marks a camera's playback as paused or resumes it.
// If paused is true and there is no existing state, one is created with
// an empty source so the caller still sees a meaningful state.
func setPlaybackPaused(camera string, paused bool) {
	playbackStatesMu.Lock()
	defer playbackStatesMu.Unlock()
	ps := playbackStates[camera]
	if ps == nil {
		return
	}
	if paused {
		now := time.Now()
		ps.PausedAt = &now
		ps.State = "paused"
	} else {
		ps.PausedAt = nil
		ps.State = "playing"
	}
}

// clearPlayback removes playback state for a camera (it has stopped).
func clearPlayback(camera string) {
	playbackStatesMu.Lock()
	delete(playbackStates, camera)
	playbackStatesMu.Unlock()
}

// clearAllPlayback removes playback state for all cameras.
func clearAllPlayback() {
	playbackStatesMu.Lock()
	playbackStates = make(map[string]*PlaybackState)
	playbackStatesMu.Unlock()
}

// getAllPlayback returns playback states for the given camera names. Cameras
// with no tracked state get an "idle" entry. If a streamSession is active,
// the current audio level is merged into the playback state for VU meter
// display.
func getAllPlayback(names []string) map[string]PlaybackState {
	playbackStatesMu.RLock()
	defer playbackStatesMu.RUnlock()

	levels := getStreamLevels()

	out := make(map[string]PlaybackState, len(names))
	for _, name := range names {
		ps := playbackStates[name]
		if ps == nil {
			out[name] = PlaybackState{State: "idle"}
		} else {
			state := *ps
			if level, ok := levels[name]; ok {
				state.Level = level
			}
			out[name] = state
		}
	}
	return out
}

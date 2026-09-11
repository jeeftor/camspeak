package api

import (
	"testing"
	"time"
)

func TestPlaybackResumeExcludesPausedTime(t *testing.T) {
	const camera = "playback-position-test"
	defer clearPlayback(camera)
	started := time.Now().Add(-20 * time.Second)
	paused := started.Add(5 * time.Second)
	playbackStatesMu.Lock()
	playbackStates[camera] = &PlaybackState{
		State: "paused", StartedAt: started, PausedAt: &paused,
	}
	playbackStatesMu.Unlock()
	setPlaybackPaused(camera, true)
	setPlaybackPaused(camera, false)
	playbackStatesMu.RLock()
	defer playbackStatesMu.RUnlock()
	state := playbackStates[camera]
	if elapsed := time.Since(state.StartedAt); elapsed < 5*time.Second || elapsed > 6*time.Second {
		t.Fatalf("elapsed playback includes paused time: %s", elapsed)
	}
	if state.PausedAt != nil || state.State != "playing" {
		t.Fatalf("unexpected resumed state: %+v", state)
	}
}

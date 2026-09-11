package api

import (
	"net/http"
	"testing"
)

func TestMalformedPauseResumeLeaveStreamsUnchanged(t *testing.T) {
	for _, action := range []string{"pause", "resume"} {
		for _, body := range []string{`{"camera":`, `{"camera":42}`, `[]`, `{"camera":"front",`} {
			t.Run(action+"/"+body, func(t *testing.T) {
				_, e, _ := setupTestHandlers(t)
				resetStreams(t)
				resetPlayback(t)
				t.Cleanup(func() { resetStreams(t); resetPlayback(t) })
				initiallyPaused := action == "resume"
				for _, camera := range []string{"front", "unrelated"} {
					addFakeStream(t, camera, "http://example/live")
					activeStreamsMu.Lock()
					activeStreams[camera].paused = initiallyPaused
					activeStreamsMu.Unlock()
				}

				res := doJSON(e, http.MethodPost, "/api/"+action, body)
				if res.Code != http.StatusBadRequest {
					t.Errorf("status = %d, want 400", res.Code)
				}
				activeStreamsMu.Lock()
				defer activeStreamsMu.Unlock()
				for camera, stream := range activeStreams {
					if stream.paused != initiallyPaused {
						t.Errorf("%s paused = %v, want %v", camera, stream.paused, initiallyPaused)
					}
				}
			})
		}
	}
}

func TestEmptyPauseResumeStillApplyToAllStreams(t *testing.T) {
	for _, body := range []string{"", `{}`} {
		t.Run(body, func(t *testing.T) {
			_, e, _ := setupTestHandlers(t)
			resetStreams(t)
			t.Cleanup(func() { resetStreams(t) })
			addFakeStream(t, "front", "http://example/live")
			for _, action := range []string{"pause", "resume"} {
				res := doJSON(e, http.MethodPost, "/api/"+action, body)
				activeStreamsMu.Lock()
				paused := activeStreams["front"].paused
				activeStreamsMu.Unlock()
				if res.Code != http.StatusOK || paused != (action == "pause") {
					t.Errorf("%s: status = %d, paused = %v", action, res.Code, paused)
				}
			}
		})
	}
}

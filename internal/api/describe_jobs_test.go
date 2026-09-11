package api

import (
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jeeftor/camspeak/internal/cameras"
	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/vision"
)

func TestDescribeJobReturnsBeforeVisionAndSurvivesRequestCancellation(t *testing.T) {
	h, e, _ := setupTestHandlers(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-release:
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"A parked car."}}]}`))
		case <-r.Context().Done():
		}
	}))
	defer model.Close()
	snapshot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = jpeg.Encode(w, image.NewRGBA(image.Rect(0, 0, 32, 32)), nil)
	}))
	defer snapshot.Close()
	t.Cleanup(h.shutdownUploads)
	camCfg := config.CameraConfig{
		Type:       "hikvision",
		IP:         "127.0.0.1",
		Enabled:    true,
		SnapMethod: "frigate",
	}
	h.cfg.Cameras["describe-job"] = camCfg
	h.cfg.FrigateURL = snapshot.URL
	h.vision = vision.NewClient(model.URL, "test", "")
	if err := h.reg.EnableCamera("describe-job", camCfg); err != nil {
		t.Fatal(err)
	}
	e.POST("/api/describe/jobs", h.StartDescribeJob)
	e.GET("/api/describe/jobs/:id", h.GetDescribeJob)
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/api/describe/jobs", strings.NewReader(`{"camera":"describe-job"}`)).
		WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { e.ServeHTTP(rec, req); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cancel()
		close(release)
		t.Fatal("start request waited for vision inference")
	}
	cancel()
	if rec.Code != http.StatusAccepted {
		t.Fatalf("start: %d %s", rec.Code, rec.Body.String())
	}
	var job DescribeJob
	if err := json.Unmarshal(rec.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("background work did not reach vision after request ended")
	}
	progress := h.describes.get(job.ID)
	if progress.Status != "running" || progress.Stage != "vision" {
		t.Fatalf("unexpected live progress: %+v", progress)
	}
	if _, ok := progress.Result["timings"].(map[string]int64)["snapshot_ms"]; !ok {
		t.Fatal("snapshot timing is not visible while vision is still running")
	}
	close(release)
	// The deliberately unavailable TTS endpoint fails after successful vision.
	deadline := time.Now().Add(2 * time.Second)
	for h.describes.get(job.ID).Status == "running" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	progress = h.describes.get(job.ID)
	if progress.Status != "error" || progress.Result["description"] != "A parked car." ||
		progress.Error == "" {
		t.Fatalf("failure lost partial result: %+v", progress)
	}
	poll := doJSON(e, http.MethodGet, "/api/describe/jobs/"+job.ID, "")
	if poll.Code != http.StatusOK || poll.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("job response: %d headers %v", poll.Code, poll.Header())
	}
}

func TestDescribeJobsSnapshotsRetentionAndBounds(t *testing.T) {
	var jobs describeJobs
	id := jobs.create("camera")
	timings := map[string]int64{"vision_ms": 23}
	jobs.update(id, "tts", map[string]any{"timings": timings})
	timings["vision_ms"] = 999
	snapshot := jobs.get(id)
	snapshot.Result["timings"].(map[string]int64)["vision_ms"] = 123
	if got := jobs.get(id).Result["timings"].(map[string]int64)["vision_ms"]; got != 23 {
		t.Fatalf("mutable timing snapshot: %d", got)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 100 {
				jobs.update(id, "tts", map[string]any{"timings": map[string]int64{"vision_ms": 24}})
				if _, err := json.Marshal(jobs.get(id)); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
	for range 40 {
		completed := jobs.create("another")
		jobs.finish(completed, map[string]any{}, nil)
	}
	if len(jobs.jobs) != 32 || jobs.get(id) == nil {
		t.Fatal("retention is unbounded or evicted an active job")
	}
	jobs.finish(id, map[string]any{}, context.Canceled)
	if jobs.get(id).Status != "canceled" {
		t.Fatal("cancellation not reported")
	}
	jobs.jobs[id].doneAt = time.Now().Add(-11 * time.Minute)
	if jobs.get(id) != nil {
		t.Fatal("expired job was retained")
	}
}

type describeLevelSpeaker struct {
	operationSpeaker
	before func()
}

func (s *describeLevelSpeaker) SendRaw(
	_ string,
	gain *cameras.GainController,
) (cameras.SendTiming, error) {
	s.before()
	gain.RecordLevel(0.6)
	return cameras.SendTiming{}, nil
}

func TestDescribeWaitsForAcceptedAudioBeforePlaying(t *testing.T) {
	cam := &describeLevelSpeaker{}
	op := beginOperation(context.Background(), "describe-level", cam, "describe", "test")
	defer op.finish()
	started := false
	op.onPlaying = func() { started = true }
	cam.before = func() {
		playbackStatesMu.Lock()
		defer playbackStatesMu.Unlock()
		if state := playbackStates[op.camera].State; state != "preparing" || started {
			t.Fatalf("premature playback: %s, callback %v", state, started)
		}
	}
	if _, err := op.send("unused", cameras.NewGainController(1)); err != nil {
		t.Fatal(err)
	}
	if !started {
		t.Fatal("accepted audio did not report playback")
	}
}

func TestDescribeTimingDoesNotCountSnapshotTwice(t *testing.T) {
	timings := NewStepTimings(4)
	timings.steps = map[string]time.Duration{
		"snap_ms": 10 * time.Millisecond, "snapshot_ms": 10 * time.Millisecond,
		"vision_ms": 20 * time.Millisecond, "send_playback_ms": time.Second,
	}
	if got := timings.TTFS(); got != 30 {
		t.Fatalf("TTFS = %d, want 30", got)
	}
}

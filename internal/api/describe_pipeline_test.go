package api

import (
	"encoding/binary"
	"encoding/json"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/jeeftor/camspeak/internal/config"
	"github.com/jeeftor/camspeak/internal/tts"
	"github.com/jeeftor/camspeak/internal/vision"
)

// setupDescribePipeline routes every upstream request to local mock servers.
// The router fetches the generated audio but never forwards it to a speaker.
func setupDescribePipeline(
	t *testing.T,
	model http.HandlerFunc,
) (*Handlers, *echo.Echo, *atomic.Int32, *atomic.Int64) {
	t.Helper()
	h, e, _ := setupTestHandlers(t)
	var syntheses atomic.Int32
	var audioBytes atomic.Int64
	wav := make([]byte, 44+1600)
	copy(wav, "RIFF")
	binary.LittleEndian.PutUint32(wav[4:], uint32(len(wav)-8))
	copy(wav[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:], 16)
	binary.LittleEndian.PutUint16(wav[20:], 1)
	binary.LittleEndian.PutUint16(wav[22:], 1)
	binary.LittleEndian.PutUint32(wav[24:], 8000)
	binary.LittleEndian.PutUint32(wav[28:], 16000)
	binary.LittleEndian.PutUint16(wav[32:], 2)
	binary.LittleEndian.PutUint16(wav[34:], 16)
	copy(wav[36:], "data")
	binary.LittleEndian.PutUint32(wav[40:], 1600)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		switch {
		case strings.HasSuffix(r.URL.Path, "/latest.jpg"):
			w.Header().Set("Content-Type", "image/jpeg")
			_ = jpeg.Encode(w, image.NewRGBA(image.Rect(0, 0, 32, 32)), nil)
		case r.URL.Path == "/v1/chat/completions":
			if model != nil {
				model(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(
				w,
				`{"choices":[{"message":{"content":"A visitor at the door."}}]}`,
			)
		case r.URL.Path == "/v1/audio/speech":
			syntheses.Add(1)
			w.Header().Set("Content-Type", "audio/wav")
			_, _ = w.Write(wav)
		case r.URL.Path == "/api/streams" && r.Method == http.MethodPost && r.URL.Query().Get("src") != "":
			src := strings.TrimPrefix(r.URL.Query().Get("src"), "ffmpeg:")
			src, _, _ = strings.Cut(src, "#")
			if !strings.HasPrefix(src, "http://127.0.0.1:") {
				t.Errorf("unexpected non-local audio URL: %s", src)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			response, err := (&http.Client{Timeout: time.Second}).Get(src)
			if err != nil {
				t.Errorf("fetch generated audio: %v", err)
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			defer response.Body.Close()
			n, err := io.Copy(io.Discard, response.Body)
			if err != nil || response.StatusCode != http.StatusOK {
				t.Errorf("audio response: status %d, error %v", response.StatusCode, err)
			}
			audioBytes.Add(n)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(upstream.Close)
	t.Cleanup(h.shutdownUploads)
	h.cfg.FrigateURL = upstream.URL
	h.cfg.PrimeSilenceMs = 0
	h.vision = vision.NewClient(upstream.URL, "test", "")
	h.tts = tts.NewClient(upstream.URL+"/v1/audio/speech", "test")
	if err := h.reg.SetRouting(upstream.URL, "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"pipeline-a", "pipeline-b", "pipeline-c"} {
		cfg := config.CameraConfig{
			Type: "go2rtc", Stream: name, IP: "127.0.0.1", Enabled: true, Gain: 1,
			SnapMethod: "frigate",
		}
		h.cfg.Cameras[name] = cfg
		if err := h.reg.EnableCamera(name, cfg); err != nil {
			t.Fatal(err)
		}
	}
	e.POST("/api/describe", h.Describe)
	e.POST("/api/describe/jobs", h.StartDescribeJob)
	e.GET("/api/describe/jobs/:id", h.GetDescribeJob)
	e.POST("/api/stop", h.Stop)
	return h, e, &syntheses, &audioBytes
}

func startPipelineJob(t *testing.T, e *echo.Echo, camera string) DescribeJob {
	t.Helper()
	response := doJSON(e, http.MethodPost, "/api/describe/jobs", `{"camera":"`+camera+`"}`)
	if response.Code != http.StatusAccepted {
		t.Fatalf("start job: %d %s", response.Code, response.Body.String())
	}
	var job DescribeJob
	if err := json.Unmarshal(response.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	if job.ID == "" || response.Header().Get("Location") != "/api/describe/jobs/"+job.ID {
		t.Fatalf("missing accepted job location: %s", response.Body.String())
	}
	return job
}

func awaitPipelineJob(t *testing.T, e *echo.Echo, id, status string) DescribeJob {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		response := doJSON(e, http.MethodGet, "/api/describe/jobs/"+id, "")
		if response.Code != http.StatusOK {
			t.Fatalf("poll job: %d %s", response.Code, response.Body.String())
		}
		var job DescribeJob
		if err := json.Unmarshal(response.Body.Bytes(), &job); err != nil {
			t.Fatal(err)
		}
		if job.Status != "running" {
			if job.Status != status || job.Stage != status {
				t.Fatalf("terminal job = %+v, want %s", job, status)
			}
			return job
		}
		if time.Now().After(deadline) {
			t.Fatalf("job did not finish: %+v", job)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestDescribePipelineCompletesAsyncAndSync(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg required")
	}
	for _, async := range []bool{true, false} {
		name := "synchronous"
		if async {
			name = "background"
		}
		t.Run(name, func(t *testing.T) {
			_, e, syntheses, audioBytes := setupDescribePipeline(t, nil)
			var result map[string]any
			if async {
				job := startPipelineJob(t, e, "pipeline-a")
				result = awaitPipelineJob(t, e, job.ID, "done").Result
			} else {
				response := doJSON(e, http.MethodPost, "/api/describe", `{"camera":"pipeline-a"}`)
				if response.Code != http.StatusOK {
					t.Fatalf("synchronous Describe: %d %s", response.Code, response.Body.String())
				}
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
			}
			if result["status"] != "ok" || result["description"] != "A visitor at the door." {
				t.Fatalf("incomplete result: %+v", result)
			}
			imageURI, _ := result["image"].(string)
			if !strings.HasPrefix(imageURI, "data:image/jpeg;base64,") {
				t.Fatalf("missing snapshot: %v", result["image"])
			}
			timings, ok := result["timings"].(map[string]any)
			if !ok {
				t.Fatalf("missing timing map: %v", result)
			}
			for _, stage := range []string{
				"snapshot_ms", "vision_ms", "tts_ms", "transcode_ms", "send_open_ms", "send_playback_ms",
			} {
				if duration, ok := timings[stage].(float64); !ok || duration < 0 {
					t.Errorf("missing or invalid %s: %v", stage, timings[stage])
				}
			}
			if _, ok := result["ttfs_ms"].(float64); !ok {
				t.Error("missing time to first audio")
			}
			if _, ok := result["total_ms"].(float64); !ok {
				t.Error("missing total timing")
			}
			if syntheses.Load() != 1 || audioBytes.Load() == 0 {
				t.Fatalf("pipeline did not deliver generated audio: syntheses %d, bytes %d",
					syntheses.Load(), audioBytes.Load())
			}
		})
	}
}

func TestDescribePipelineCancellationDuringVision(t *testing.T) {
	for _, action := range []string{"stop", "shutdown"} {
		t.Run(action, func(t *testing.T) {
			entered := make(chan struct{})
			canceled := make(chan struct{})
			h, e, syntheses, audioBytes := setupDescribePipeline(
				t,
				func(_ http.ResponseWriter, r *http.Request) {
					close(entered)
					<-r.Context().Done()
					close(canceled)
				},
			)
			job := startPipelineJob(t, e, "pipeline-a")
			select {
			case <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("Describe did not reach vision")
			}
			if action == "stop" {
				response := doJSON(e, http.MethodPost, "/api/stop", `{"camera":"pipeline-a"}`)
				if response.Code != http.StatusOK {
					t.Fatalf("stop: %d %s", response.Code, response.Body.String())
				}
			} else {
				h.shutdownUploads()
			}
			result := awaitPipelineJob(t, e, job.ID, "canceled")
			if result.Error == "" {
				t.Error("cancellation has no explanation")
			}
			select {
			case <-canceled:
			case <-time.After(time.Second):
				t.Fatal("vision request remained open after cancellation")
			}
			if syntheses.Load() != 0 || audioBytes.Load() != 0 {
				t.Fatal("canceled vision proceeded to speech or playback")
			}
		})
	}
}

func TestDescribePipelineWorkerLimit(t *testing.T) {
	entered := make(chan struct{}, 2)
	h, e, _, _ := setupDescribePipeline(t, func(_ http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		<-r.Context().Done()
	})
	first := startPipelineJob(t, e, "pipeline-a")
	second := startPipelineJob(t, e, "pipeline-b")
	for range 2 {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("two active jobs did not reach vision")
		}
	}
	response := doJSON(e, http.MethodPost, "/api/describe/jobs", `{"camera":"pipeline-c"}`)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("third job: %d %s, want 503", response.Code, response.Body.String())
	}
	h.shutdownUploads()
	awaitPipelineJob(t, e, first.ID, "canceled")
	awaitPipelineJob(t, e, second.ID, "canceled")
	response = doJSON(e, http.MethodPost, "/api/describe/jobs", `{"camera":"pipeline-c"}`)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("job accepted after shutdown: %d", response.Code)
	}
}

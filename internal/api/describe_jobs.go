package api

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

// DescribeJob is a snapshot of a background description and speaker operation.
type DescribeJob struct {
	ID        string         `json:"id"`
	Camera    string         `json:"camera"`
	Status    string         `json:"status"`
	Stage     string         `json:"stage"`
	ElapsedMs int64          `json:"elapsed_ms"`
	Result    map[string]any `json:"result"`
	Error     string         `json:"error,omitempty"`
	startedAt time.Time
	doneAt    time.Time
}

// describeJobs reuses the bounded upload worker lifecycle, with isolated job state.
type describeJobs struct {
	worker uploadWorker
	mu     sync.Mutex
	jobs   map[string]*DescribeJob
}

func cloneDescribeResult(result map[string]any) map[string]any {
	copy := maps.Clone(result)
	if timings, ok := copy["timings"].(map[string]int64); ok {
		copy["timings"] = maps.Clone(timings)
	}
	return copy
}

func (j *describeJobs) create(camera string) string {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.jobs == nil {
		j.jobs = make(map[string]*DescribeJob)
	}
	for id, job := range j.jobs {
		if !job.doneAt.IsZero() && time.Since(job.doneAt) > 10*time.Minute {
			delete(j.jobs, id)
		}
	}
	// Bound retained results as well as active workers. Never evict active jobs.
	for len(j.jobs) >= 32 {
		var oldest *DescribeJob
		for _, job := range j.jobs {
			if !job.doneAt.IsZero() && (oldest == nil || job.doneAt.Before(oldest.doneAt)) {
				oldest = job
			}
		}
		if oldest == nil {
			break
		}
		delete(j.jobs, oldest.ID)
	}
	id := rand.Text()
	j.jobs[id] = &DescribeJob{
		ID: id, Camera: camera, Status: "running", Stage: "snapshot",
		Result: map[string]any{}, startedAt: time.Now(),
	}
	return id
}

func (j *describeJobs) update(id, stage string, result map[string]any) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if job := j.jobs[id]; job != nil {
		job.Stage = stage
		job.Result = cloneDescribeResult(result)
	}
}

func (j *describeJobs) finish(id string, result map[string]any, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	job := j.jobs[id]
	job.Result = cloneDescribeResult(result)
	job.Status = "done"
	if err != nil {
		job.Status = "error"
		job.Error = err.Error()
		var httpErr *echo.HTTPError
		if errors.As(err, &httpErr) {
			job.Error = fmt.Sprint(httpErr.Message)
		}
		if errors.Is(err, context.Canceled) {
			job.Status = "canceled"
			job.Error = "Describe was stopped or replaced by another action."
		}
	}
	job.Stage = job.Status
	job.doneAt = time.Now()
	job.ElapsedMs = job.doneAt.Sub(job.startedAt).Milliseconds()
	if job.Result != nil {
		job.Result["total_ms"] = job.ElapsedMs
	}
}

func (j *describeJobs) get(id string) *DescribeJob {
	j.mu.Lock()
	defer j.mu.Unlock()
	job := j.jobs[id]
	if job == nil {
		return nil
	}
	if !job.doneAt.IsZero() && time.Since(job.doneAt) > 10*time.Minute {
		delete(j.jobs, id)
		return nil
	}
	copy := *job
	copy.Result = cloneDescribeResult(job.Result)
	if copy.Status == "running" {
		copy.ElapsedMs = time.Since(copy.startedAt).Milliseconds()
	}
	return &copy
}

// StartDescribeJob accepts work without keeping the WAN request open during inference.
func (h *Handlers) StartDescribeJob(c echo.Context) error {
	var req describeRequest
	if err := c.Bind(&req); err != nil || req.Camera == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "camera required")
	}
	if err := validateRequestGain(req.Gain); err != nil {
		return err
	}
	cfg := h.configSnapshot()
	client := h.visionClient()
	if client == nil {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "vision model not configured")
	}
	cam, err := h.reg.GetForPlayback(req.Camera)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, err.Error())
	}
	workerCtx, ok := h.describes.worker.acquire()
	if !ok {
		return echo.NewHTTPError(http.StatusServiceUnavailable, "Describe is busy; try again shortly")
	}
	ctx, cancel := context.WithTimeout(workerCtx, 10*time.Minute)
	op, err := h.beginPlaybackOperation(ctx, req.Camera, cam, "describe", "vision description")
	if err != nil {
		cancel()
		h.describes.worker.release()
		return echo.NewHTTPError(http.StatusConflict, err.Error())
	}
	id := h.describes.create(req.Camera)
	log := h.logger(c)
	go func() {
		defer h.describes.worker.release()
		defer cancel()
		defer op.finish()
		result, runErr := h.runDescribe(
			op,
			req,
			cfg,
			client,
			log,
			func(stage string, result map[string]any) {
				h.describes.update(id, stage, result)
			},
		)
		if op.ctx.Err() != nil {
			runErr = op.ctx.Err()
		}
		h.describes.finish(id, result, runErr)
	}()
	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set("Location", "/api/describe/jobs/"+id)
	return c.JSON(http.StatusAccepted, h.describes.get(id))
}

// GetDescribeJob returns completed timings and the current stage using a short request.
func (h *Handlers) GetDescribeJob(c echo.Context) error {
	c.Response().Header().Set("Cache-Control", "no-store")
	job := h.describes.get(c.Param("id"))
	if job == nil {
		return echo.NewHTTPError(http.StatusNotFound, "Describe job expired or does not exist")
	}
	return c.JSON(http.StatusOK, job)
}

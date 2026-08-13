package api

import (
	"sync"
	"testing"
	"time"

	"github.com/jeeftor/camspeak/internal/library"
)

// UploadJobStatus values.
const (
	JobTranscoding = "transcoding"
	JobSaving      = "saving"
	JobDone        = "done"
	JobError       = "error"
)

// UploadJob tracks the progress of an async file upload + transcode.
type UploadJob struct {
	ID        string          `json:"id"`
	Status    string          `json:"status"`  // transcoding, saving, done, error
	Percent   float64         `json:"percent"` // 0–100, -1 = indeterminate
	Step      string          `json:"step"`    // human-readable current step
	Name      string          `json:"name"`
	Category  string          `json:"category"`
	Filename  string          `json:"filename"`
	Error     string          `json:"error,omitempty"`
	Preset    *library.Preset `json:"preset,omitempty"`
	StartedAt time.Time       `json:"started_at"`
	DoneAt    *time.Time      `json:"done_at,omitempty"`
}

var (
	uploadJobs      = make(map[string]*UploadJob)
	uploadJobsMu    sync.RWMutex
	uploadJobsSeq   uint64
	uploadJobsSeqMu sync.Mutex
)

// nextJobID returns a monotonically increasing job ID.
func nextJobID() string {
	uploadJobsSeqMu.Lock()
	uploadJobsSeq++
	id := uploadJobsSeq
	uploadJobsSeqMu.Unlock()
	return formatJobID(id)
}

func formatJobID(seq uint64) string {
	// Simple, sortable ID without external deps.
	return time.Now().Format("20060102-150405") + "-" + itoa(seq)
}

// itoa is a tiny uint64 → string converter to avoid strconv in this hot path.
func itoa(n uint64) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// newUploadJob creates and registers a job, returning the job pointer.
func newUploadJob(name, category, filename string) *UploadJob {
	job := &UploadJob{
		ID:        nextJobID(),
		Status:    JobTranscoding,
		Percent:   0,
		Step:      "Transcoding",
		Name:      name,
		Category:  category,
		Filename:  filename,
		StartedAt: time.Now(),
	}
	uploadJobsMu.Lock()
	uploadJobs[job.ID] = job
	uploadJobsMu.Unlock()
	return job
}

// getUploadJob returns a job by ID, or nil if not found.
func getUploadJob(id string) *UploadJob {
	uploadJobsMu.RLock()
	job := uploadJobs[id]
	uploadJobsMu.RUnlock()
	return job
}

// updateUploadJob updates a job's progress. Safe to call from goroutines.
func updateUploadJob(id string, percent float64, step string) {
	uploadJobsMu.Lock()
	job := uploadJobs[id]
	if job != nil {
		job.Percent = percent
		job.Step = step
	}
	uploadJobsMu.Unlock()
}

// completeUploadJob marks a job as done with the resulting preset.
func completeUploadJob(id string, preset *library.Preset) {
	uploadJobsMu.Lock()
	job := uploadJobs[id]
	if job != nil {
		now := time.Now()
		job.Status = JobDone
		job.Step = "Done"
		job.Percent = 100
		job.Preset = preset
		job.DoneAt = &now
	}
	uploadJobsMu.Unlock()
}

// failUploadJob marks a job as failed with an error message.
func failUploadJob(id, errMsg string) {
	uploadJobsMu.Lock()
	job := uploadJobs[id]
	if job != nil {
		job.Status = JobError
		job.Step = "Error"
		job.Error = errMsg
		now := time.Now()
		job.DoneAt = &now
	}
	uploadJobsMu.Unlock()
}

// cleanupOldUploadJobs removes jobs older than the given duration.
func cleanupOldUploadJobs(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	uploadJobsMu.Lock()
	for id, job := range uploadJobs {
		if job.DoneAt != nil && job.DoneAt.Before(cutoff) {
			delete(uploadJobs, id)
		}
	}
	uploadJobsMu.Unlock()
}

// resetUploadJobs clears all jobs (for testing).
func resetUploadJobs(t *testing.T) {
	t.Helper()
	uploadJobsMu.Lock()
	uploadJobs = make(map[string]*UploadJob)
	uploadJobsMu.Unlock()
}

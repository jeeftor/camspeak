package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jeeftor/camspeak/internal/library"
)

// resetUploadJobs clears the tracker for isolated tests.
func resetUploadJobs(t *testing.T) {
	t.Helper()
	uploadJobsMu.Lock()
	uploadJobs = make(map[string]*UploadJob)
	uploadJobsMu.Unlock()
}

func TestUploadSnapshotConcurrentEncoding(t *testing.T) {
	resetUploadJobs(t)
	job := newUploadJob("audio", "uploads", "audio.wav")
	var wg sync.WaitGroup
	wg.Go(func() {
		for i := range 1000 {
			updateUploadJob(job.ID, float64(i%100), "Transcoding")
		}
		completeUploadJob(job.ID, &library.Preset{Meta: library.Meta{Name: "audio"}})
	})
	for range 1000 {
		if _, err := json.Marshal(getUploadJob(job.ID)); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
	snapshot := getUploadJob(job.ID)
	snapshot.Preset.Name = "changed"
	*snapshot.DoneAt = time.Time{}
	if current := getUploadJob(job.ID); current.Preset.Name != "audio" || current.DoneAt.IsZero() {
		t.Fatal("returned snapshot aliases stored job")
	}
}

func TestUploadWorkerCapacityAndShutdown(t *testing.T) {
	h := &Handlers{}
	first, ok := h.uploads.acquire()
	if !ok {
		t.Fatal("first upload rejected")
	}
	_, ok = h.uploads.acquire()
	if !ok {
		t.Fatal("second upload rejected")
	}
	if _, ok := h.uploads.acquire(); ok {
		t.Fatal("third simultaneous upload must be rejected")
	}
	finished := make(chan struct{})
	go func() {
		h.shutdownUploads()
		close(finished)
	}()
	select {
	case <-first.Done():
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel workers")
	}
	select {
	case <-finished:
		t.Fatal("shutdown returned before workers finished")
	default:
	}
	h.uploads.release()
	h.uploads.release()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not finish after workers exited")
	}
	if _, ok := h.uploads.acquire(); ok {
		t.Fatal("worker accepted new upload after shutdown")
	}
}

func TestUploadRejectsInvalidIdentifierBeforeTranscode(t *testing.T) {
	h, e, _ := setupTestHandlers(t)
	e.POST("/api/library/upload", h.UploadPreset)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("name", "../outside"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/library/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	h.shutdownUploads()
}

func TestUploadRejectsOversizeRequest(t *testing.T) {
	h, e, _ := setupTestHandlers(t)
	e.POST("/api/library/upload", h.UploadPreset)
	// Stream a large part without allocating a matching request buffer.
	reader, writer := io.Pipe()
	multipartWriter := multipart.NewWriter(writer)
	go func() {
		defer writer.Close()
		part, err := multipartWriter.CreateFormFile("file", "large.wav")
		if err != nil {
			return
		}
		chunk := make([]byte, 1<<20)
		for range 65 {
			if _, err := part.Write(chunk); err != nil {
				return
			}
		}
		_ = multipartWriter.Close()
	}()
	defer reader.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/library/upload", reader)
	req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	h.shutdownUploads()
}

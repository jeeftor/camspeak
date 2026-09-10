package vision

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDescribeCancellationReachesServer(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(entered)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	client := NewClient(server.URL, "vision", "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := client.DescribeContext(ctx, []byte{0xFF, 0xD8}, "image/jpeg", "test"); done <- err }()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("request failed before reaching server: %v", err)
	case <-time.After(time.Second):
		t.Fatal("vision request did not reach server")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("vision request did not cancel")
	}
}

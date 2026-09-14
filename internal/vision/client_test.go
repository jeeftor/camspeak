package vision

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

// Reasoning models emit chain-of-thought into reasoning_content, sharing the
// completion-token budget with the answer. When the budget runs out mid-
// reasoning the content is empty — that must be reported as reasoning
// exhaustion, not a generic empty response.
func TestDescribeReportsReasoningExhaustion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{"finish_reason": "length", "index": 0, "message": {
				"content": "",
				"reasoning_content": "Got it, let's analyze the image..."
			}}],
			"usage": {"completion_tokens": 150}
		}`))
	}))
	defer server.Close()
	client := NewClient(server.URL, "vision", "")
	_, err := client.Describe([]byte{0xFF, 0xD8}, "image/jpeg", "test")
	if err == nil || !strings.Contains(err.Error(), "reasoning") {
		t.Fatalf("err = %v, want a reasoning-exhaustion error", err)
	}
	if strings.Contains(err.Error(), "empty response") {
		t.Fatalf("reasoning exhaustion reported as empty response: %v", err)
	}
}

// An answer alongside reasoning_content returns the answer; the chain of
// thought is never spoken.
func TestDescribeReturnsAnswerAlongsideReasoning(t *testing.T) {
	var maxTokens float64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		maxTokens, _ = req["max_tokens"].(float64)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{"finish_reason": "stop", "index": 0, "message": {
				"content": "A dog on the porch.",
				"reasoning_content": "Let me look at the image..."
			}}],
			"usage": {"completion_tokens": 80}
		}`))
	}))
	defer server.Close()
	client := NewClient(server.URL, "vision", "")
	got, err := client.Describe([]byte{0xFF, 0xD8}, "image/jpeg", "test")
	if err != nil {
		t.Fatal(err)
	}
	if got != "A dog on the porch." {
		t.Fatalf("description = %q", got)
	}
	if maxTokens < 512 {
		t.Fatalf("max_tokens = %v, want room for reasoning + answer", maxTokens)
	}
}

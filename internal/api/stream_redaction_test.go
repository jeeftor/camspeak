package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	clog "github.com/charmbracelet/log"
	"github.com/jeeftor/camspeak/internal/logging"
)

func TestStreamURLTransportAndDisplayAreSeparate(t *testing.T) {
	raw := "https://alice:private-password@example.test/live?token=private-token#private-fragment"
	transport, err := resolveStreamURL(raw)
	if err != nil || transport != raw {
		t.Fatalf("transport URL changed: %q, %v", transport, err)
	}
	display := streamDisplayURL(raw)
	if display != "https://example.test/live" {
		t.Fatalf("unsafe display URL: %q", display)
	}
	replay := playbackReplay("/api/play-stream", map[string]any{"url": raw}, -1)
	if !replay.Redacted || replay.Body["url"] != display {
		t.Fatalf("authenticated replay must be marked redacted: %#v", replay)
	}
	if got := streamDisplayURL("https://example.test/%zz?token=private-token"); got != "[invalid URL removed]" {
		t.Fatalf("invalid URL not redacted: %q", got)
	}
}

func TestStreamStderrRedactsURLs(t *testing.T) {
	var output bytes.Buffer
	logger := logging.New("api-test", clog.DebugLevel)
	logger.SetOutput(&output)
	line := "Opening 'https://alice:private-password@example.test/live?token=private-token#private-fragment' for reading\n"
	logStreamStderr(io.NopCloser(strings.NewReader(line)), logger, "redaction-test", nil)
	got := output.String()
	for _, secret := range []string{"alice", "private-password", "private-token", "private-fragment"} {
		if strings.Contains(got, secret) {
			t.Fatalf("stderr leaked %s: %s", secret, got)
		}
	}
	if !strings.Contains(got, "example.test/live") {
		t.Fatalf("diagnostic lost useful endpoint: %s", got)
	}
}

func TestPreparedStreamPublishesOnlySafeURL(t *testing.T) {
	h, _, _ := setupTestHandlers(t)
	const camera = "stream-redaction"
	const raw = "https://alice:private-password@example.test/live?token=private-token#private-fragment"
	var output bytes.Buffer
	logger := logging.New("api-test", clog.InfoLevel)
	logger.SetOutput(&output)
	op := beginOperation(context.Background(), camera, &operationSpeaker{}, "stream", "")
	finished := make(chan struct{})
	release := op.releasePriority
	op.releasePriority = func() { release(); close(finished) }
	events := h.events.subscribe()
	defer h.events.unsubscribe(events)

	// Keep the supervisor before ffmpeg creation while inspecting published
	// state. Cancel before releasing the lock, so no network is contacted.
	h.cfgMu.Lock()
	defer func() {
		op.cancel()
		h.cfgMu.Unlock()
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
			t.Error("stream supervisor did not finish")
		}
	}()
	if err := h.startPreparedStream(logger, op, raw, raw, -1); err != nil {
		t.Fatal(err)
	}
	state := getAllPlayback([]string{camera})[camera]
	if state.Detail != "https://example.test/live" {
		t.Fatalf("playback URL was not redacted: %q", state.Detail)
	}
	pausedURL, _, ok := pauseStream(camera)
	if !ok || pausedURL != state.Detail {
		t.Fatalf("pause response URL was not redacted: %q", pausedURL)
	}
	resumedURL, _, ok := resumeStream(camera)
	if !ok || resumedURL != state.Detail {
		t.Fatalf("resume response URL was not redacted: %q", resumedURL)
	}
	ev := <-events
	encoded, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"alice", "private-password", "private-token", "private-fragment"} {
		if strings.Contains(output.String(), secret) || strings.Contains(string(encoded), secret) {
			t.Fatalf("stream start log/event leaked %s", secret)
		}
	}
	if ev.Replay == nil || !ev.Replay.Redacted {
		t.Fatal("authenticated replay must be marked redacted")
	}
}

func TestStreamDiagnosticErrorPreservesCancellation(t *testing.T) {
	err := streamDiagnosticError{
		fmt.Errorf("Get https://alice:password@example.test/live?token=secret: %w", context.Canceled),
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatal("redaction lost cancellation identity")
	}
	if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "password") {
		t.Fatalf("error leaked credentials: %s", err)
	}
}

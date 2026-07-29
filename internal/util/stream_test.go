package util

import (
	"bytes"
	"io"
	"sync"
	"testing"
	"time"
)

func TestCopyAt8kBps(t *testing.T) {
	// 1600 bytes at 8000 bytes/sec = 200ms of audio.
	// CopyAt8kBps writes in 800-byte chunks (100ms each), so 2 chunks = ~200ms.
	data := bytes.Repeat([]byte{0xAB}, 1600)

	pr, pw := io.Pipe()
	var out bytes.Buffer
	var stopped bool
	var mu sync.Mutex

	done := make(chan error, 1)
	go func() {
		done <- CopyAt8kBps(&out, pr, &stopped, &mu)
	}()

	// Write all data then close the writer end so the reader gets EOF.
	go func() {
		_, _ = pw.Write(data)
		pw.Close()
	}()

	start := time.Now()
	err := <-done
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("CopyAt8kBps returned error: %v", err)
	}
	if !bytes.Equal(out.Bytes(), data) {
		t.Errorf("output mismatch: got %d bytes, want %d bytes", out.Len(), len(data))
	}
	// Should take at least ~200ms (2 chunks of 100ms) but allow generous lower bound.
	if elapsed < 150*time.Millisecond {
		t.Errorf("elapsed = %v, expected at least ~200ms (pacing)", elapsed)
	}
}

func TestCopyAt8kBpsStoppedFlag(t *testing.T) {
	// Write more than one chunk worth of data, then set stopped mid-stream.
	data := bytes.Repeat([]byte{0x01}, 2400) // 3 chunks

	pr, pw := io.Pipe()
	var out bytes.Buffer
	var stopped bool
	var mu sync.Mutex

	done := make(chan error, 1)
	go func() {
		done <- CopyAt8kBps(&out, pr, &stopped, &mu)
	}()

	go func() {
		_, _ = pw.Write(data)
	}()

	// Wait a bit for at least one chunk to be written, then stop.
	time.Sleep(150 * time.Millisecond)
	mu.Lock()
	stopped = true
	mu.Unlock()
	pw.Close()

	err := <-done
	if err == nil {
		t.Fatal("expected error from stopped flag, got nil")
	}
	if err.Error() != "stream interrupted" {
		t.Errorf("error = %q, want %q", err.Error(), "stream interrupted")
	}
}

func TestCopyAt8kBpsEOF(t *testing.T) {
	// Verify that EOF on reader causes a clean return (nil error).
	data := bytes.Repeat([]byte{0xCD}, 800) // exactly 1 chunk

	pr, pw := io.Pipe()
	var out bytes.Buffer
	var stopped bool
	var mu sync.Mutex

	done := make(chan error, 1)
	go func() {
		done <- CopyAt8kBps(&out, pr, &stopped, &mu)
	}()

	go func() {
		_, _ = pw.Write(data)
		pw.Close() // triggers EOF on reader
	}()

	err := <-done
	if err != nil {
		t.Errorf("CopyAt8kBps returned error on EOF: %v, want nil", err)
	}
	if !bytes.Equal(out.Bytes(), data) {
		t.Errorf("output mismatch: got %d bytes, want %d bytes", out.Len(), len(data))
	}
}

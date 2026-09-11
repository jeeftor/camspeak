package util

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// digestPeer reads a probe and responds with exactly the supplied bytes.
func digestPeer(t *testing.T, response string, stall bool) (string, <-chan struct{}) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
		reader := bufio.NewReader(conn)
		if _, err := http.ReadRequest(reader); err != nil {
			return
		}
		if _, err := io.WriteString(conn, response); err != nil {
			return
		}
		close(ready)
		if stall {
			_, _ = io.Copy(io.Discard, reader)
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-done
	})
	return listener.Addr().String(), ready
}

func TestDigestAuthResponses(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantAuth bool
		wantErr  bool
	}{
		{"no authentication", "HTTP/1.1 200 OK\r\n\r\n", false, false},
		{
			"digest challenge",
			"HTTP/1.1 401 Unauthorized\r\n" +
				"WWW-Authenticate: Digest realm=\"camera\", nonce=\"test\", qop=\"auth\"\r\n\r\n",
			true,
			false,
		},
		{"incomplete headers", "HTTP/1.1 401 Unauthorized\r\nWWW-Authenticate:", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, _ := digestPeer(t, tt.response, false)
			auth, err := performDigestAuth(context.Background(), addr, "/audio", "user", "pass")
			if (err != nil) != tt.wantErr {
				t.Fatalf("error=%v, wantErr=%v", err, tt.wantErr)
			}
			if strings.HasPrefix(auth, "Digest ") != tt.wantAuth {
				t.Fatalf("authorization present=%v, want=%v", auth != "", tt.wantAuth)
			}
		})
	}
}

func TestDigestAuthCancellationInterruptsStalledRead(t *testing.T) {
	for _, response := range []string{"", "HTTP/1.1 401 Unauthorized\r\n"} {
		t.Run(response, func(t *testing.T) {
			addr, ready := digestPeer(t, response, true)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() {
				_, err := performDigestAuth(ctx, addr, "/audio", "user", "pass")
				result <- err
			}()
			select {
			case <-ready:
			case <-time.After(time.Second):
				t.Fatal("digest probe did not reach the test peer")
			}
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("error=%v, want context cancellation", err)
				}
			case <-time.After(time.Second):
				t.Fatal("digest read ignored cancellation")
			}
		})
	}
}

func TestDigestAuthHonorsCallerDeadline(t *testing.T) {
	addr, _ := digestPeer(t, "HTTP/1.1 401 Unauthorized\r\n", true)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := performDigestAuth(ctx, addr, "/audio", "user", "pass")
	if err == nil {
		t.Fatal("stalled response was treated as unauthenticated success")
	}
	if time.Since(start) > time.Second {
		t.Fatal("digest read exceeded the caller deadline")
	}
}

func TestDigestAuthHasDefaultHandshakeDeadline(t *testing.T) {
	if testing.Short() {
		t.Skip("checks the five-second production timeout")
	}
	addr, _ := digestPeer(t, "", true)
	start := time.Now()
	_, err := performDigestAuth(context.Background(), addr, "/audio", "user", "pass")
	elapsed := time.Since(start)
	if err == nil || elapsed < 4*time.Second || elapsed > 7*time.Second {
		t.Fatalf(
			"stalled handshake: error=%v elapsed=%v; want timeout after five seconds",
			err,
			elapsed,
		)
	}
}

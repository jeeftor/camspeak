package cameras

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestGo2rtcReportsLevelsOnlyAfterSuccessfulAcknowledgment(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			entered := make(chan struct{})
			releaseHeaders := make(chan struct{})
			releaseBody := make(chan struct{})
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Query().Get("src") == "" {
						w.WriteHeader(http.StatusOK)
						return
					}
					close(entered)
					select {
					case <-releaseHeaders:
					case <-r.Context().Done():
						return
					}
					w.Header().Set("Content-Length", "1")
					w.WriteHeader(status)
					w.(http.Flusher).Flush()
					select {
					case <-releaseBody:
						_, _ = w.Write([]byte("x"))
					case <-r.Context().Done():
					}
				}),
			)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer func() { cancel(); server.Close() }()
			file := filepath.Join(t.TempDir(), "audio.mulaw")
			if err := os.WriteFile(file, make([]byte, 8000), 0o600); err != nil {
				t.Fatal(err)
			}
			var levels atomic.Int32
			firstLevel := make(chan struct{}, 1)
			gain := NewGainController(1).WithLevelSink(func(float64) {
				levels.Add(1)
				select {
				case firstLevel <- struct{}{}:
				default:
				}
			})
			client := NewGo2rtcClient(server.URL, "test", "127.0.0.1", "127.0.0.1", "test")
			done := make(chan error, 1)
			go func() {
				_, err := client.SendRawContext(ctx, file, gain)
				done <- err
			}()
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("request did not reach go2rtc")
			}
			// The old simulated ticker announced playback while these headers stalled.
			select {
			case <-firstLevel:
				t.Fatal("announced audio before go2rtc acknowledged the request")
			case <-time.After(150 * time.Millisecond):
			}
			close(releaseHeaders)
			if status == http.StatusOK {
				select {
				case <-firstLevel:
				case <-ctx.Done():
					t.Fatal("successful go2rtc acknowledgment did not publish an audio level")
				}
			}
			close(releaseBody)
			select {
			case err := <-done:
				if (err != nil) != (status != http.StatusOK) {
					t.Fatalf("error=%v for HTTP %d", err, status)
				}
			case <-ctx.Done():
				t.Fatal("send did not finish")
			}
			if status != http.StatusOK && levels.Load() != 0 {
				t.Fatal("rejected go2rtc request still published audio levels")
			}
		})
	}
}

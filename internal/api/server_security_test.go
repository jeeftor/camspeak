package api

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestRateLimitSharedAcrossRequests(t *testing.T) {
	e := echo.New()
	e.Use(newRateLimitMiddleware())
	e.GET("/api/test", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	e.POST("/api/stop", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	before := runtime.NumGoroutine()
	limited := 0
	for range 100 {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.RemoteAddr = "192.0.2.1:10000"
		res := httptest.NewRecorder()
		e.ServeHTTP(res, req)
		if res.Code == http.StatusTooManyRequests {
			limited++
			if res.Header().Get("Retry-After") == "" {
				t.Fatal("limited response needs retry guidance")
			}
		}
	}
	if limited == 0 {
		t.Fatal("consecutive same-client requests were never limited")
	}
	if delta := runtime.NumGoroutine() - before; delta > 5 {
		t.Fatalf("request handling leaked goroutines: %d", delta)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/stop", nil)
	req.RemoteAddr = "192.0.2.1:10000"
	res := httptest.NewRecorder()
	e.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("emergency stop was limited: %d", res.Code)
	}
}

func TestShutdownInterruptsUnfinishedUpload(t *testing.T) {
	h, e, _ := setupTestHandlers(t)
	e.POST("/api/library/upload", h.UploadPreset)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	e.Listener = listener
	s := &Server{echo: e, handlers: h}
	serveDone := make(chan error, 1)
	go func() { serveDone <- e.Start("") }()
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	defer e.Close()
	_, err = fmt.Fprintf(
		conn,
		"POST /api/library/upload HTTP/1.1\r\nHost: localhost\r\nContent-Type: multipart/form-data; boundary=test\r\nContent-Length: 10000\r\n\r\n--test\r\n",
	)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		h.uploads.mu.Lock()
		active := h.uploads.active
		h.uploads.mu.Unlock()
		if active > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("upload handler did not start")
		}
		time.Sleep(time.Millisecond)
	}
	stopped := make(chan error, 1)
	go func() { stopped <- s.Stop() }()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown blocked on incomplete upload")
	}
	<-serveDone
}

func TestBrowserOriginBoundary(t *testing.T) {
	for _, tc := range []struct {
		name, origin, configured string
		status                   int
	}{
		{"non-browser", "", "", http.StatusOK},
		{"same-origin", "http://example.com", "", http.StatusOK},
		{"cross-origin", "https://untrusted.example", "", http.StatusForbidden},
		{"opaque-origin", "null", "", http.StatusForbidden},
		{"trusted-frontend", "https://frontend.example", "https://frontend.example", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := echo.New()
			e.Use(browserOriginMiddleware(tc.configured))
			e.POST("/api/test", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
			req := httptest.NewRequest(http.MethodPost, "http://example.com/api/test", nil)
			req.Header.Set(echo.HeaderOrigin, tc.origin)
			res := httptest.NewRecorder()
			e.ServeHTTP(res, req)
			if res.Code != tc.status {
				t.Fatalf("status = %d, want %d", res.Code, tc.status)
			}
		})
	}
}

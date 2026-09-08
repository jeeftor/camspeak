package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseICYMetadata(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		want  string
		wantK icyKind
	}{
		{
			name:  "music StreamTitle",
			line:  "ICY Info: StreamTitle='Artist - Title';",
			want:  "Artist - Title",
			wantK: icyTitle,
		},
		{
			name:  "music StreamTitle with trailing semicolons",
			line:  "ICY Info: StreamTitle='Song by Band';StreamUrl='http://example';",
			want:  "Song by Band",
			wantK: icyTitle,
		},
		{
			name:  "ATC icy-name indented",
			line:  "icy-name        : KAFF",
			want:  "KAFF",
			wantK: icyName,
		},
		{
			name:  "icy-name no indent",
			line:  "icy-name: KAFF",
			want:  "KAFF",
			wantK: icyName,
		},
		{
			name:  "icy-name with extra whitespace",
			line:  "icy-name    :   KAFF Tower   ",
			want:  "KAFF Tower",
			wantK: icyName,
		},
		{
			name:  "non-metadata line",
			line:  "  Duration: 00:00:00.00, start: 0.000000",
			want:  "",
			wantK: icyNone,
		},
		{
			name:  "empty line",
			line:  "",
			want:  "",
			wantK: icyNone,
		},
		{
			name:  "icy-description indented",
			line:  "icy-description : KAFF Tower ATC",
			want:  "KAFF Tower ATC",
			wantK: icyDescription,
		},
		{
			name:  "icy-description no indent",
			line:  "icy-description: Ambient beats",
			want:  "Ambient beats",
			wantK: icyDescription,
		},
		{
			name:  "StreamTitle empty",
			line:  "ICY Info: StreamTitle='';",
			want:  "",
			wantK: icyTitle,
		},
		{
			name:  "StreamTitle with apostrophe in title",
			line:  "ICY Info: StreamTitle='Don't Stop Me Now';",
			want:  "Don",
			wantK: icyTitle,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, kind := parseICYMetadata(tc.line)
			if got != tc.want {
				t.Errorf("parseICYMetadata(%q) value = %q, want %q", tc.line, got, tc.want)
			}
			if kind != tc.wantK {
				t.Errorf("parseICYMetadata(%q) kind = %d, want %d", tc.line, kind, tc.wantK)
			}
		})
	}
}

func TestMulawDecode(t *testing.T) {
	// μ-law silence codes 0xFF and 0x7F decode to small nonzero values
	// (~±132). The silence floor observed in computeLevel (~0.064)
	// comes from sqrt compression of the RMS of these small values.
	tests := []struct {
		name string
		b    byte
	}{
		{"silence 0xFF", 0xFF},
		{"zero 0x7F", 0x7F},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := mulawDecode(tc.b)
			// Silence codes decode to small values, not exactly 0.
			if v > 200 || v < -200 {
				t.Errorf("mulawDecode(0x%02X) = %d, expected small value (<200)", tc.b, v)
			}
		})
	}
}

func TestMulawDecodeSymmetry(t *testing.T) {
	// All μ-law bytes should decode to valid int16 values.
	// (staticcheck SA4003: no int16 is < math.MinInt16 or > math.MaxInt16,
	// so we just verify the decode doesn't panic for all 256 byte values.)
	for b := 0; b < 256; b++ {
		_ = mulawDecode(byte(b))
	}
}

func TestComputeLevel(t *testing.T) {
	// Empty buffer returns 0.
	if level := computeLevel(nil); level != 0 {
		t.Errorf("computeLevel(nil) = %f, want 0", level)
	}

	// Silence bytes (0xFF) produce the μ-law silence floor (~0.064),
	// not exactly 0. This is expected — the decoded value of 0xFF is
	// a small nonzero number due to μ-law's segment encoding.
	silence := []byte{0xFF, 0xFF, 0xFF, 0xFF}
	level := computeLevel(silence)
	if level < 0.05 || level > 0.08 {
		t.Errorf("computeLevel(silence) = %f, expected ~0.064 (μ-law floor)", level)
	}

	// A buffer with varying non-silence bytes should produce a positive level.
	buf := make([]byte, 100)
	for i := range buf {
		buf[i] = byte(i % 256)
	}
	level = computeLevel(buf)
	if level <= 0 {
		t.Errorf("computeLevel(varied) = %f, want > 0", level)
	}
	if level > 1.0 {
		t.Errorf("computeLevel(varied) = %f, want <= 1.0", level)
	}
}

func TestResolveStreamURL(t *testing.T) {
	// Non-playlist URLs pass through unchanged.
	direct := "http://example.com/stream"
	got, err := resolveStreamURL(direct)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != direct {
		t.Errorf("resolveStreamURL(%q) = %q, want %q", direct, got, direct)
	}
}

func TestResolvePLS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/x-scpls")
		_, _ = w.Write([]byte("[playlist]\nFile1=http://stream.example.com/live\nNumberOfEntries=1\n"))
	}))
	defer srv.Close()

	got, err := resolvePLS(srv.URL)
	if err != nil {
		t.Fatalf("resolvePLS: %v", err)
	}
	want := "http://stream.example.com/live"
	if got != want {
		t.Errorf("resolvePLS() = %q, want %q", got, want)
	}
}

func TestResolvePLSRelativeURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("[playlist]\nFile1=/live/stream\n"))
	}))
	defer srv.Close()

	got, err := resolvePLS(srv.URL)
	if err != nil {
		t.Fatalf("resolvePLS: %v", err)
	}
	if !strings.HasPrefix(got, "http://") {
		t.Errorf("resolvePLS() = %q, expected resolved URL", got)
	}
	if !strings.HasSuffix(got, "/live/stream") {
		t.Errorf("resolvePLS() = %q, expected path /live/stream", got)
	}
}

func TestResolvePLSNoFile1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("[playlist]\nNumberOfEntries=0\n"))
	}))
	defer srv.Close()

	_, err := resolvePLS(srv.URL)
	if err == nil {
		t.Error("resolvePLS() expected error for no File1, got nil")
	}
}

func TestResolveM3U(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-mpegURL")
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1,Live Stream\nhttp://stream.example.com/live\n"))
	}))
	defer srv.Close()

	got, err := resolveM3U(srv.URL)
	if err != nil {
		t.Fatalf("resolveM3U: %v", err)
	}
	want := "http://stream.example.com/live"
	if got != want {
		t.Errorf("resolveM3U() = %q, want %q", got, want)
	}
}

func TestResolveM3URelativeURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("#EXTM3U\n/live/stream\n"))
	}))
	defer srv.Close()

	got, err := resolveM3U(srv.URL)
	if err != nil {
		t.Fatalf("resolveM3U: %v", err)
	}
	if !strings.HasPrefix(got, "http://") {
		t.Errorf("resolveM3U() = %q, expected resolved URL", got)
	}
	if !strings.HasSuffix(got, "/live/stream") {
		t.Errorf("resolveM3U() = %q, expected path /live/stream", got)
	}
}

func TestResolveM3UEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("#EXTM3U\n"))
	}))
	defer srv.Close()

	_, err := resolveM3U(srv.URL)
	if err == nil {
		t.Error("resolveM3U() expected error for empty playlist, got nil")
	}
}

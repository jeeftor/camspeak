package logging

import (
	"testing"

	clog "github.com/charmbracelet/log"
)

func TestNew(t *testing.T) {
	l := New("test", clog.InfoLevel)
	if l == nil {
		t.Fatal("New returned nil")
	}
	if l.GetLevel() != clog.InfoLevel {
		t.Fatalf("level = %v, want InfoLevel", l.GetLevel())
	}
}

func TestNewDebugEnablesCaller(t *testing.T) {
	l := New("test", clog.DebugLevel)
	if l == nil {
		t.Fatal("New returned nil")
	}
	if l.GetLevel() != clog.DebugLevel {
		t.Fatalf("level = %v, want DebugLevel", l.GetLevel())
	}
}

func TestColorForPrefix(t *testing.T) {
	cases := []struct {
		prefix string
		want   string
	}{
		{"api", "#38BDF8"},
		{"airplay", "#A78BFA"},
		{"airplay[backyard]", "#A78BFA"},
		{"mqtt", "#FB923C"},
		{"unknown", "#94A3B8"},
	}
	for _, tc := range cases {
		t.Run(tc.prefix, func(t *testing.T) {
			got := string(colorForPrefix(tc.prefix))
			if got != tc.want {
				t.Fatalf("colorForPrefix(%q) = %q, want %q", tc.prefix, got, tc.want)
			}
		})
	}
}

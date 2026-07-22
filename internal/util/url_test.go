package util

import (
	"net/url"
	"testing"
)

func TestRedactURL(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no credentials",
			input:    "http://example.com/audio.wav",
			expected: "http://example.com/audio.wav",
		},
		{
			name:     "with user and password",
			input:    "http://user:pass@example.com/audio.wav",
			expected: "http://example.com/audio.wav",
		},
		{
			name:     "with user only",
			input:    "http://user@example.com/audio.wav",
			expected: "http://example.com/audio.wav",
		},
		{
			name:     "with query and fragment",
			input:    "http://user:secret@example.com/audio.wav?foo=bar#baz",
			expected: "http://example.com/audio.wav?foo=bar#baz",
		},
		{
			name:     "nil url",
			input:    "",
			expected: "",
		},
		{
			name:     "RedactURLString helper",
			input:    "http://user:pass@example.com/audio.wav",
			expected: "http://example.com/audio.wav",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "RedactURLString helper" {
				got := RedactURLString(tc.input)
				if got != tc.expected {
					t.Fatalf("RedactURLString(%q) = %q, want %q", tc.input, got, tc.expected)
				}
				return
			}
			var u *url.URL
			if tc.input != "" {
				var err error
				u, err = url.Parse(tc.input)
				if err != nil {
					t.Fatalf("parse url: %v", err)
				}
			}
			got := RedactURL(u)
			if got != tc.expected {
				t.Fatalf("RedactURL(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

// Package util contains small, shared helpers used across camspeak.
package util

import "net/url"

// RedactURL returns a URL string with any embedded credentials removed.
// It only strips the userinfo portion (user:pass@host); the rest of the URL
// is preserved for diagnostics.
func RedactURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	redacted := *u
	redacted.User = nil
	return redacted.String()
}

// RedactURLString parses s and returns it with any embedded credentials removed.
// If s cannot be parsed, it is returned unchanged.
func RedactURLString(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return s
	}
	return RedactURL(u)
}

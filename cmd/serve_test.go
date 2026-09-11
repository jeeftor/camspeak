package cmd

import (
	"strings"
	"testing"
)

func TestVersionBanner(t *testing.T) {
	plain := versionBanner("v4.4.0", false)
	if strings.Contains(plain, "\x1b[") || !strings.Contains(plain, "[ VERSION v4.4.0 ]") {
		t.Fatalf("unexpected plain banner: %q", plain)
	}
	colored := versionBanner("v4.4.0", true)
	if !strings.Contains(colored, "\x1b[") || !strings.Contains(colored, "[ VERSION v4.4.0 ]") {
		t.Fatalf("missing highlighted version: %q", colored)
	}
}

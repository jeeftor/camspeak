package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRemoteCheckReadOnlyAndPrivate(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer private-token" {
			t.Error("wrong method or authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/health" {
			fmt.Fprint(w, `{"version":"test"}`)
		} else {
			fmt.Fprint(w, `[]`)
		}
	}))
	defer server.Close()
	var output bytes.Buffer
	if err := checkRemote(context.Background(), &output, server.URL, "private-token"); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || strings.Contains(output.String(), "private-token") {
		t.Fatal("unexpected request or leaked token")
	}
}

func TestRemoteCheckRejectsProxyPagesAndRedirects(t *testing.T) {
	for _, status := range []int{200, 302, 401, 403} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Location", "/login")
				w.WriteHeader(status)
				fmt.Fprint(w, "<html>private-cookie</html>")
			}))
			defer server.Close()
			var output bytes.Buffer
			err := checkRemote(context.Background(), &output, server.URL, "private-token")
			if err == nil || calls != 1 {
				t.Fatalf("expected one failed request, got %d %v", calls, err)
			}
			if strings.Contains(err.Error(), "private") {
				t.Fatal("response/credential leaked")
			}
		})
	}
}

func TestRemoteCheckRejectsUnsafeServers(t *testing.T) {
	for _, server := range []string{"http://example.com", "https://user:pass@example.com", "https://example.com?token=secret"} {
		if err := checkRemote(context.Background(), &bytes.Buffer{}, server, "secret"); err == nil {
			t.Fatal("unsafe server accepted")
		}
	}
}

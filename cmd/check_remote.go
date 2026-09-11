package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(newCheckRemoteCommand())
}

func newCheckRemoteCommand() *cobra.Command {
	var server, tokenFile string
	command := &cobra.Command{
		Use: "check-remote", Short: "Read-only remote API and authentication check (no speaker playback)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if server == "" {
				server = os.Getenv("CAMSPEAK_SERVER")
			}
			token := strings.TrimSpace(os.Getenv("CAMSPEAK_ACCESS_TOKEN"))
			if tokenFile != "" {
				file, err := os.Open(tokenFile)
				if err != nil {
					return fmt.Errorf("cannot open token file")
				}
				defer file.Close()
				info, err := file.Stat()
				if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
					return fmt.Errorf(
						"token file must be a regular file readable only by its owner (chmod 600)",
					)
				}
				data, err := io.ReadAll(io.LimitReader(file, 16385))
				if err != nil || len(data) > 16384 {
					return fmt.Errorf("cannot read token file or token exceeds 16 KiB")
				}
				token = strings.TrimSpace(string(data))
				if token == "" {
					return fmt.Errorf("token file is empty")
				}
			}
			return checkRemote(cmd.Context(), cmd.OutOrStdout(), server, token)
		},
		Args: cobra.NoArgs,
	}
	command.Flags().StringVar(&server, "server", "", "CamSpeak HTTPS base URL (or CAMSPEAK_SERVER)")
	command.Flags().
		StringVar(&tokenFile, "token-file", "", "Private file containing a provider-issued bearer token; otherwise CAMSPEAK_ACCESS_TOKEN")
	return command
}

func checkRemote(ctx context.Context, output io.Writer, server, token string) error {
	u, err := url.Parse(server)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("provide a server URL without credentials, query, or fragment")
	}
	ip := net.ParseIP(u.Hostname())
	loopback := u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback())
	if u.Scheme != "https" && (u.Scheme != "http" || !loopback) {
		return fmt.Errorf("HTTPS is required except for loopback testing")
	}
	if strings.ContainsAny(token, "\r\n") {
		return fmt.Errorf("token must contain a single line")
	}
	client := &http.Client{
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	for _, path := range []string{"/api/health", "/api/cameras"} {
		endpoint := *u
		endpoint.Path = strings.TrimRight(u.Path, "/") + path
		endpoint.RawPath = ""
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return fmt.Errorf("cannot create remote check request")
		}
		req.Header.Set("Accept", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		started := time.Now()
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf(
				"%s: connection failed; check DNS, TLS trust, network access, or timeout",
				path,
			)
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf(
				"%s: HTTP %d; redirects are not followed—check proxy access policy and provider-issued token",
				path,
				resp.StatusCode,
			)
		}
		mediaType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
		if readErr != nil || len(data) > 1<<20 || mediaType != "application/json" || !json.Valid(data) {
			return fmt.Errorf(
				"%s: expected bounded JSON, received an invalid response (possibly a proxy login page)",
				path,
			)
		}
		if path == "/api/health" {
			var health struct {
				Version string `json:"version"`
			}
			if json.Unmarshal(data, &health) != nil || health.Version == "" {
				return fmt.Errorf("%s: missing CamSpeak version", path)
			}
		} else {
			var cameras []json.RawMessage
			if json.Unmarshal(data, &cameras) != nil || strings.TrimSpace(string(data)) == "null" {
				return fmt.Errorf("%s: expected camera array", path)
			}
		}
		fmt.Fprintf(output, "%s: OK (%dms)\n", path, time.Since(started).Milliseconds())
	}
	fmt.Fprintln(
		output,
		"Read-only checks passed. No audio was played. This does not verify audible output or enforce server authentication.",
	)
	return nil
}

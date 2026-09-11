package util

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/icholy/digest"
)

// PerformDigestAuth does the HTTP 401 challenge/response handshake for the
// given path on ip:80 and returns the Authorization header value.
// Returns ("", nil) if the endpoint does not require authentication.
func PerformDigestAuth(ip, path, user, pass string) (string, error) {
	return PerformDigestAuthContext(context.Background(), ip, path, user, pass)
}

// PerformDigestAuthContext bounds the entire handshake to five seconds and
// closes its connection when ctx is canceled, including while reading headers.
func PerformDigestAuthContext(ctx context.Context, ip, path, user, pass string) (string, error) {
	return performDigestAuth(ctx, net.JoinHostPort(ip, "80"), path, user, pass)
}

func performDigestAuth(
	ctx context.Context,
	addr, path, user, pass string,
) (auth string, err error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	defer func() {
		if err != nil && ctx.Err() != nil {
			err = fmt.Errorf("digest handshake: %w", ctx.Err())
		}
	}()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return "", fmt.Errorf("setting digest handshake deadline: %w", err)
	}

	probe := fmt.Sprintf("PUT %s HTTP/1.1\r\nHost: %s\r\nContent-Length: 0\r\n\r\n", path, addr)
	if _, err := conn.Write([]byte(probe)); err != nil {
		return "", err
	}

	r := bufio.NewReader(io.LimitReader(conn, 64*1024))
	if _, err := r.ReadString('\n'); err != nil {
		return "", err
	}
	var wwwAuth string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("reading digest response headers: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(line), "www-authenticate:") {
			wwwAuth = strings.TrimSpace(line[len("www-authenticate:"):])
		}
	}
	if wwwAuth == "" {
		return "", nil
	}
	chal, err := digest.FindChallenge(http.Header{"Www-Authenticate": []string{wwwAuth}})
	if err != nil {
		return "", err
	}
	cred, err := digest.Digest(chal, digest.Options{
		Method:   http.MethodPut,
		URI:      path,
		Username: user,
		Password: pass,
	})
	if err != nil {
		return "", err
	}
	return cred.String(), nil
}

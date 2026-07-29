package util

import (
	"bufio"
	"fmt"
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
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, "80"), 5*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	probe := fmt.Sprintf("PUT %s HTTP/1.1\r\nHost: %s\r\nContent-Length: 0\r\n\r\n", path, ip)
	if _, err := conn.Write([]byte(probe)); err != nil {
		return "", err
	}

	r := bufio.NewReader(conn)
	if _, err := r.ReadString('\n'); err != nil {
		return "", err
	}
	var wwwAuth string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			break
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

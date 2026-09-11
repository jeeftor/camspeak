package cameras

import (
	"errors"
	"io"
	"net"
	"strings"
	"testing"
)

type audioWriteConn struct {
	net.Conn
	writes    int
	bodyError error
	response  io.Reader
}

func (c *audioWriteConn) Write(data []byte) (int, error) {
	c.writes++
	if c.writes > 1 && c.bodyError != nil {
		return 0, c.bodyError
	}
	return len(data), nil
}

func (c *audioWriteConn) Read(data []byte) (int, error) {
	return c.response.Read(data)
}

func TestHikvisionReportsLevelOnlyAfterAudioWrite(t *testing.T) {
	for _, fails := range []bool{false, true} {
		name := "successful write"
		if fails {
			name = "failed write"
		}
		t.Run(name, func(t *testing.T) {
			conn := &audioWriteConn{response: strings.NewReader("HTTP/1.1 200 OK\r\n")}
			if fails {
				conn.bodyError = errors.New("speaker disconnected")
			}
			levels := 0
			gain := NewGainController(1).WithLevelSink(func(float64) {
				levels++
				if conn.writes < 2 {
					t.Error("reported audio before writing the first chunk")
				}
			})
			client := NewHikvisionClient("camera", "", "", 1, "test")
			_, err := client.sendAudioWithAuth(conn, "/audio", "camera", "", []byte{0}, gain)
			if fails {
				if err == nil || levels != 0 {
					t.Fatalf(
						"failed write: err=%v, levels=%d; want error and no level",
						err,
						levels,
					)
				}
			} else if err != nil || levels != 1 {
				t.Fatalf("successful write: err=%v, levels=%d; want one level", err, levels)
			}
		})
	}
}

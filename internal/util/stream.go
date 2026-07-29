package util

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// Deadliner is implemented by *os.File (returned by cmd.StdoutPipe).
type Deadliner interface {
	SetReadDeadline(t time.Time) error
}

// CopyAt8kBps copies r → w paced at 8000 bytes/sec (G.711 mulaw real-time rate).
// It polls the stopped flag every 200 ms so SendRaw preemption is responsive
// even when the reader has no data (e.g. AirPlay is selected but silent).
// If r implements Deadliner, read deadlines are used to avoid blocking forever.
func CopyAt8kBps(w io.Writer, r io.Reader, stopped *bool, mu *sync.Mutex) error {
	const chunkSize = 800 // 100ms at 8000 bytes/sec
	const interval = 100 * time.Millisecond
	const readTimeout = 200 * time.Millisecond

	dl, _ := r.(Deadliner)
	buf := make([]byte, chunkSize)
	pending := 0
	next := time.Now()

	for {
		mu.Lock()
		s := *stopped
		mu.Unlock()
		if s {
			return fmt.Errorf("stream interrupted")
		}

		if dl != nil {
			_ = dl.SetReadDeadline(time.Now().Add(readTimeout))
		}
		n, err := r.Read(buf[pending:])
		pending += n

		if pending >= chunkSize {
			if dl != nil {
				_ = dl.SetReadDeadline(time.Time{}) // clear deadline before write
			}
			if _, werr := w.Write(buf[:chunkSize]); werr != nil {
				mu.Lock()
				s := *stopped
				mu.Unlock()
				if s {
					return fmt.Errorf("stream interrupted")
				}
				return werr
			}
			copy(buf, buf[chunkSize:pending])
			pending -= chunkSize
			next = next.Add(interval)
			if sleep := time.Until(next); sleep > 0 {
				time.Sleep(sleep)
			}
		}

		if err != nil {
			if dl != nil {
				type timeouter interface{ Timeout() bool }
				if t, ok := err.(timeouter); ok && t.Timeout() {
					continue // just a read deadline — check stopped and retry
				}
			}
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return nil
			}
			return err
		}
	}
}

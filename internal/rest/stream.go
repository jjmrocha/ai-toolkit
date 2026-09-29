package rest

import (
	"bufio"
	"errors"
	"io"
	"iter"
	"strings"
	"sync/atomic"
	"time"
)

// IdleTimeout is how long a streamed body may go without data before
// [WithIdleTimeout] gives up.
const IdleTimeout = 5 * time.Minute

// WithIdleTimeout wraps body so that a read that waits longer than timeout
// closes body and fails with [ErrIdleTimeout]. Each read that returns restarts
// the timer, so a body that keeps sending is never cut off. Closing the
// returned reader closes body.
func WithIdleTimeout(body io.ReadCloser, timeout time.Duration) io.ReadCloser {
	r := &idleReader{body: body, timeout: timeout}

	r.timer = time.AfterFunc(timeout, func() {
		r.expired.Store(true)
		_ = body.Close()
	})

	return r
}

type idleReader struct {
	body    io.ReadCloser
	timeout time.Duration
	timer   *time.Timer
	expired atomic.Bool
}

func (r *idleReader) Read(p []byte) (int, error) {
	n, err := r.body.Read(p)
	if r.expired.Load() {
		return n, ErrIdleTimeout
	}

	r.timer.Reset(r.timeout)

	return n, err
}

func (r *idleReader) Close() error {
	r.timer.Stop()
	return r.body.Close()
}

// Events reads body as server-sent events and yields each event's data in
// order, joining a multi-line event's lines with newlines. Comments,
// keep-alives and fields other than data are skipped. Iteration ends with the
// body. A read error is yielded once, after the events before it, and ends
// iteration.
func Events(body io.Reader) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		reader := bufio.NewReader(body)
		var data []string

		for {
			line, err := reader.ReadString('\n')
			if err != nil && !errors.Is(err, io.EOF) {
				yield("", err)
				return
			}

			line = strings.TrimRight(line, "\r\n")
			if value, ok := strings.CutPrefix(line, "data:"); ok {
				data = append(data, strings.TrimPrefix(value, " "))
			}

			if (line == "" || err != nil) && len(data) > 0 {
				if !yield(strings.Join(data, "\n"), nil) {
					return
				}
				data = nil
			}

			if err != nil {
				return
			}
		}
	}
}

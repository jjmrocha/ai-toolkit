package rest

import (
	"errors"
	"io"
	"iter"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWithIdleTimeout(t *testing.T) {
	t.Run("reads the whole body when data keeps arriving", func(t *testing.T) {
		// given
		pr, pw := io.Pipe()
		go func() {
			_, _ = io.WriteString(pw, "hello ")
			_, _ = io.WriteString(pw, "world")
			_ = pw.Close()
		}()
		body := WithIdleTimeout(pr, time.Second)
		defer func() { _ = body.Close() }()
		// when
		result, err := io.ReadAll(body)
		// then
		assert.NoError(t, err)
		assert.Equal(t, "hello world", string(result))
	})

	t.Run("restarts the timeout on every read", func(t *testing.T) {
		// given
		pr, pw := io.Pipe()
		go func() {
			for range 6 {
				time.Sleep(50 * time.Millisecond)
				_, _ = io.WriteString(pw, "x")
			}
			_ = pw.Close()
		}()
		body := WithIdleTimeout(pr, 200*time.Millisecond)
		defer func() { _ = body.Close() }()
		// when
		result, err := io.ReadAll(body)
		// then
		assert.NoError(t, err)
		assert.Equal(t, "xxxxxx", string(result))
	})

	t.Run("returns ErrIdleTimeout when the body goes quiet", func(t *testing.T) {
		// given
		pr, pw := io.Pipe()
		defer func() { _ = pw.Close() }()
		go func() {
			_, _ = io.WriteString(pw, "partial")
		}()
		body := WithIdleTimeout(pr, 20*time.Millisecond)
		defer func() { _ = body.Close() }()
		// when
		result, err := io.ReadAll(body)
		// then
		assert.ErrorIs(t, err, ErrIdleTimeout)
		assert.Equal(t, "partial", string(result))
	})
}

func TestEvents(t *testing.T) {
	t.Run("yields the data of each event in order", func(t *testing.T) {
		// given
		body := strings.NewReader("data: first\n\ndata: second\n\n")
		// when
		result, err := collectEvents(Events(body))
		// then
		assert.NoError(t, err)
		assert.Equal(t, []string{"first", "second"}, result)
	})

	t.Run("skips comments and fields other than data", func(t *testing.T) {
		// given
		body := strings.NewReader(": OPENROUTER PROCESSING\n\nevent: ping\nid: 7\nretry: 100\ndata: {\"type\":\"ping\"}\n\n")
		// when
		result, err := collectEvents(Events(body))
		// then
		assert.NoError(t, err)
		assert.Equal(t, []string{`{"type":"ping"}`}, result)
	})

	t.Run("joins multi-line data with newlines", func(t *testing.T) {
		// given
		body := strings.NewReader("data: line one\ndata: line two\n\n")
		// when
		result, err := collectEvents(Events(body))
		// then
		assert.NoError(t, err)
		assert.Equal(t, []string{"line one\nline two"}, result)
	})

	t.Run("accepts CRLF line endings and data without a space", func(t *testing.T) {
		// given
		body := strings.NewReader("data:first\r\n\r\ndata: second\r\n\r\n")
		// when
		result, err := collectEvents(Events(body))
		// then
		assert.NoError(t, err)
		assert.Equal(t, []string{"first", "second"}, result)
	})

	t.Run("yields a final event not followed by a blank line", func(t *testing.T) {
		// given
		body := strings.NewReader("data: first\n\ndata: last")
		// when
		result, err := collectEvents(Events(body))
		// then
		assert.NoError(t, err)
		assert.Equal(t, []string{"first", "last"}, result)
	})

	t.Run("reads data lines longer than a bufio buffer", func(t *testing.T) {
		// given
		long := strings.Repeat("x", 200_000)
		body := strings.NewReader("data: " + long + "\n\n")
		// when
		result, err := collectEvents(Events(body))
		// then
		assert.NoError(t, err)
		assert.Equal(t, []string{long}, result)
	})

	t.Run("yields nothing for an empty body", func(t *testing.T) {
		// when
		result, err := collectEvents(Events(strings.NewReader("")))
		// then
		assert.NoError(t, err)
		assert.Empty(t, result)
	})

	t.Run("yields the read error after the events before it", func(t *testing.T) {
		// given
		expected := errors.New("connection reset")
		body := io.MultiReader(strings.NewReader("data: first\n\n"), iotest.ErrReader(expected))
		// when
		result, err := collectEvents(Events(body))
		// then
		assert.ErrorIs(t, err, expected)
		assert.Equal(t, []string{"first"}, result)
	})

	t.Run("stops reading when the caller breaks", func(t *testing.T) {
		// given
		body := strings.NewReader("data: first\n\ndata: second\n\n")
		var result []string
		// when
		for data := range Events(body) {
			result = append(result, data)
			break
		}
		// then
		assert.Equal(t, []string{"first"}, result)
	})
}

func collectEvents(events iter.Seq2[string, error]) ([]string, error) {
	var result []string
	for data, err := range events {
		if err != nil {
			return result, err
		}
		result = append(result, data)
	}

	return result, nil
}

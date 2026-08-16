package mcp

import (
	"context"
	"os/exec"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func startStdioTransport(t testing.TB, command string, args ...string) *stdioTransport {
	t.Helper()

	c, err := newStdioTransport(command, args, nil)
	if err != nil {
		t.Fatalf("newStdioTransport(%q): unexpected error: %v", command, err)
	}
	t.Cleanup(c.Close)

	return c
}

func collectLines(c *stdioTransport) []string {
	var lines []string
	for line := range c.Reader() {
		lines = append(lines, line)
	}

	return lines
}

func TestStdioTransportWrite(t *testing.T) {
	t.Run("delivers the message to the server stdin", func(t *testing.T) {
		// given: cat echoes back whatever it reads
		c := startStdioTransport(t, "cat")
		// when
		err := c.Write(t.Context(), "hello")
		// then
		require.NoError(t, err)
		expected := "hello"
		assert.Equal(t, expected, <-c.Reader())
	})

	t.Run("rejects a message containing a newline", func(t *testing.T) {
		// given: the framing check runs before any channel is touched
		c := &stdioTransport{}
		// when
		err := c.Write(t.Context(), "one\ntwo")
		// then
		assert.ErrorIs(t, err, ErrInvalidMessage)
	})

	t.Run("returns an error once the connection is closed", func(t *testing.T) {
		// given
		c := startStdioTransport(t, "cat")
		c.Close()
		// when
		err := c.Write(t.Context(), "hello")
		// then
		assert.ErrorIs(t, err, ErrProcessClosed)
	})

	t.Run("returns the context error when the context is cancelled", func(t *testing.T) {
		// given: no writer goroutine, so only the context can unblock the send
		c := &stdioTransport{outgoing: make(chan string)}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		// when
		err := c.Write(ctx, "hello")
		// then
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestStdioTransportReader(t *testing.T) {
	t.Run("delivers each line without its newline", func(t *testing.T) {
		// given
		c := startStdioTransport(t, "sh", "-c", `printf 'first\nsecond\n'`)
		// when
		result := collectLines(c)
		// then
		expected := []string{"first", "second"}
		assert.Equal(t, expected, result)
	})

	t.Run("skips blank lines", func(t *testing.T) {
		// given
		c := startStdioTransport(t, "sh", "-c", `printf 'first\n\nsecond\n'`)
		// when
		result := collectLines(c)
		// then
		expected := []string{"first", "second"}
		assert.Equal(t, expected, result)
	})

	t.Run("stops reading a line longer than maxMessageBytes", func(t *testing.T) {
		// given: 1 MiB + 8 KiB of output with no newline, so the cap is exceeded and
		size := maxMessageBytes + 8*1024
		script := "head -c " + strconv.Itoa(size) + ` /dev/zero | tr '\0' 'a'`
		c := startStdioTransport(t, "sh", "-c", script)
		// when
		result := collectLines(c)
		// then
		assert.Empty(t, result)
	})

	t.Run("stops a server whose output can no longer be read", func(t *testing.T) {
		// given: an over-long line, followed by a server that stays alive on stdin
		size := maxMessageBytes + 8*1024
		script := "head -c " + strconv.Itoa(size) + ` /dev/zero | tr '\0' 'a'; cat > /dev/null`
		c := startStdioTransport(t, "sh", "-c", script)
		// when: the reader gives up on the stream
		result := collectLines(c)
		// then: the transport stops reporting a connection it cannot read from
		assert.Empty(t, result)
		assert.Eventually(t, func() bool { return !c.Running() }, 5*time.Second, 10*time.Millisecond)
	})

	t.Run("closes the channel once the server exits", func(t *testing.T) {
		// given
		c := startStdioTransport(t, "sh", "-c", "exit 0")
		// when
		_, ok := <-c.Reader()
		// then
		assert.False(t, ok)
	})

	t.Run("closes the channel on Close while the output is undrained", func(t *testing.T) {
		// given: the server writes without anyone reading, so the reader parks on send
		c := startStdioTransport(t, "sh", "-c", "echo one; echo two; cat > /dev/null")
		// when
		c.Close()
		// then
		_, ok := <-c.Reader()
		assert.False(t, ok)
	})
}

func TestStdioTransportClose(t *testing.T) {
	t.Run("returns as soon as the server exits on stdin close", func(t *testing.T) {
		// given: cat stays alive on stdin and exits on EOF
		c := startStdioTransport(t, "cat")
		// when
		start := time.Now()
		c.Close()
		// then: no signal is needed, so Close returns well before the first timeout
		assert.Less(t, time.Since(start), stopTimeout)
	})

	t.Run("is idempotent across concurrent calls", func(t *testing.T) {
		// given
		const goroutines = 10
		c := startStdioTransport(t, "cat")
		var wg sync.WaitGroup
		// when
		for range goroutines {
			wg.Go(c.Close)
		}
		wg.Wait()
		// then
		assert.False(t, c.Running())
	})
}

func TestStdioTransportRunning(t *testing.T) {
	t.Run("returns true while the server process is running", func(t *testing.T) {
		// given
		c := startStdioTransport(t, "cat")
		// when
		result := c.Running()
		// then
		assert.True(t, result)
	})

	t.Run("returns false after the server process exits on its own", func(t *testing.T) {
		// given: wait for the watcher to reap the self-exited process
		c := startStdioTransport(t, "sh", "-c", "exit 0")
		<-c.exited
		// when
		result := c.Running()
		// then
		assert.False(t, result)
	})
}

func TestNewStdioTransportOnExit(t *testing.T) {
	t.Run("invokes the callback with the exit error", func(t *testing.T) {
		// given
		disconnected := make(chan error, 1)
		c, err := newStdioTransport("sh", []string{"-c", "exit 3"}, func(err error) { disconnected <- err })
		require.NoError(t, err)
		t.Cleanup(c.Close)
		// when
		result := <-disconnected
		// then
		var exitErr *exec.ExitError
		require.ErrorAs(t, result, &exitErr)
		expected := 3
		assert.Equal(t, expected, exitErr.ExitCode())
	})

	t.Run("stops the transport when no callback is registered", func(t *testing.T) {
		// given: a server that exits on its own, with nothing to notify
		c, err := newStdioTransport("sh", []string{"-c", "exit 0"}, nil)
		require.NoError(t, err)
		t.Cleanup(c.Close)
		// when
		<-c.exited
		// then
		assert.False(t, c.Running())
	})
}

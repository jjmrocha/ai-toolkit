package helper

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func startProcess(t testing.TB, command string, args ...string) *Process {
	t.Helper()

	p, err := NewProcess(ProcessConfig{Path: command, Args: args, AllowInput: true})
	if err != nil {
		t.Fatalf("NewProcess(%q): unexpected error: %v", command, err)
	}

	t.Cleanup(p.Close)

	return p
}

func collectLines(p *Process) []string {
	var lines []string
	for line := range p.Output() {
		lines = append(lines, line)
	}

	return lines
}

func TestProcessWrite(t *testing.T) {
	t.Run("delivers the message to the process stdin", func(t *testing.T) {
		// given: cat echoes back whatever it reads
		p := startProcess(t, "cat")
		// when
		err := p.Write(t.Context(), "hello")
		// then
		require.NoError(t, err)
		expected := "hello"
		assert.Equal(t, expected, <-p.Output())
	})

	t.Run("rejects a message containing a newline", func(t *testing.T) {
		// given: the framing check runs before any channel is touched
		p := &Process{allowInput: true}
		// when
		err := p.Write(t.Context(), "one\ntwo")
		// then
		assert.ErrorIs(t, err, ErrInvalidMessage)
	})

	t.Run("returns an error once the process is closed", func(t *testing.T) {
		// given
		p := startProcess(t, "cat")
		p.Close()
		// when
		err := p.Write(t.Context(), "hello")
		// then
		assert.ErrorIs(t, err, ErrProcessClosed)
	})

	t.Run("rejects input when AllowInput is not set", func(t *testing.T) {
		// given
		p, err := NewProcess(ProcessConfig{Path: "cat"})
		require.NoError(t, err)
		t.Cleanup(p.Close)
		// when
		err = p.Write(t.Context(), "hello")
		// then
		assert.ErrorIs(t, err, ErrInputNotAllowed)
	})

	t.Run("returns the context error when the context is cancelled", func(t *testing.T) {
		// given: no writer goroutine, so only the context can unblock the send
		p := &Process{allowInput: true, outgoing: make(chan string)}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		// when
		err := p.Write(ctx, "hello")
		// then
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestProcessOutput(t *testing.T) {
	t.Run("delivers each line without its newline", func(t *testing.T) {
		// given
		p := startProcess(t, "sh", "-c", `printf 'first\nsecond\n'`)
		// when
		result := collectLines(p)
		// then
		expected := []string{"first", "second"}
		assert.Equal(t, expected, result)
	})

	t.Run("keeps blank lines", func(t *testing.T) {
		// given
		p := startProcess(t, "sh", "-c", `printf 'first\n\nsecond\n'`)
		// when
		result := collectLines(p)
		// then
		expected := []string{"first", "", "second"}
		assert.Equal(t, expected, result)
	})

	t.Run("delivers a line of any length", func(t *testing.T) {
		// given: far more than bufio's default, with no newline anywhere
		size := 1024*1024 + 8*1024
		script := "head -c " + strconv.Itoa(size) + ` /dev/zero | tr '\0' 'a'`
		p := startProcess(t, "sh", "-c", script)
		// when
		result := collectLines(p)
		// then
		require.Len(t, result, 1)
		assert.Len(t, result[0], size)
	})

	t.Run("closes the channel once the process exits", func(t *testing.T) {
		// given
		p := startProcess(t, "sh", "-c", "exit 0")
		// when
		_, ok := <-p.Output()
		// then
		assert.False(t, ok)
	})

	t.Run("closes the channel on Close while the output is undrained", func(t *testing.T) {
		// given: the process writes without anyone reading, so the reader parks on send
		p := startProcess(t, "sh", "-c", "echo one; echo two; cat > /dev/null")
		// when
		p.Close()
		// then
		_, ok := <-p.Output()
		assert.False(t, ok)
	})

	t.Run("merges stderr in write order when IncludeStderr is set", func(t *testing.T) {
		// given
		p, err := NewProcess(ProcessConfig{
			Path:          "sh",
			Args:          []string{"-c", "echo out1; echo err1 >&2; echo out2; echo err2 >&2"},
			IncludeStderr: true,
		})
		require.NoError(t, err)
		t.Cleanup(p.Close)
		// when
		result := collectLines(p)
		// then
		expected := []string{"out1", "err1", "out2", "err2"}
		assert.Equal(t, expected, result)
	})

	t.Run("discards stderr when IncludeStderr is not set", func(t *testing.T) {
		// given
		p := startProcess(t, "sh", "-c", "echo out1; echo err1 >&2")
		// when
		result := collectLines(p)
		// then
		expected := []string{"out1"}
		assert.Equal(t, expected, result)
	})

	t.Run("delivers every line to a consumer that pauses", func(t *testing.T) {
		// given: output stays buffered in the pipe while the consumer sleeps
		const lines = 200
		script := `i=0; while [ $i -lt 200 ]; do echo "line$i"; i=$((i+1)); done`
		p := startProcess(t, "sh", "-c", script)
		var result []string
		// when
		for line := range p.Output() {
			if len(result) == 0 {
				time.Sleep(200 * time.Millisecond)
			}

			result = append(result, line)
		}
		// then
		assert.Len(t, result, lines)
	})

	t.Run("closes stdin at once when AllowInput is not set", func(t *testing.T) {
		// given: cat blocks forever on an open stdin, and exits on EOF
		p, err := NewProcess(ProcessConfig{Path: "cat"})
		require.NoError(t, err)
		t.Cleanup(p.Close)
		// when
		result := collectLines(p)
		// then
		assert.Empty(t, result)
	})

	t.Run("runs the process in Dir", func(t *testing.T) {
		// given
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "data.txt"), []byte("payload"), 0o600))
		p, err := NewProcess(ProcessConfig{Path: "cat", Args: []string{"data.txt"}, Dir: dir})
		require.NoError(t, err)
		t.Cleanup(p.Close)
		// when
		result := collectLines(p)
		// then
		expected := []string{"payload"}
		assert.Equal(t, expected, result)
	})
}

func TestProcessClose(t *testing.T) {
	t.Run("returns as soon as the process exits on stdin close", func(t *testing.T) {
		// given: cat stays alive on stdin and exits on EOF
		p := startProcess(t, "cat")
		// when
		start := time.Now()
		p.Close()
		// then: no signal is needed, so Close returns well before the first timeout
		assert.Less(t, time.Since(start), defaultStopTimeout)
	})

	t.Run("returns when the output pipe is shared with stderr", func(t *testing.T) {
		// given: the reader only reaches EOF if the parent's write end was closed
		p, err := NewProcess(ProcessConfig{Path: "cat", IncludeStderr: true, AllowInput: true})
		require.NoError(t, err)
		t.Cleanup(p.Close)
		// when
		start := time.Now()
		p.Close()
		// then
		assert.Less(t, time.Since(start), defaultStopTimeout)
	})

	t.Run("is idempotent across concurrent calls", func(t *testing.T) {
		// given
		const goroutines = 10
		p := startProcess(t, "cat")
		var wg sync.WaitGroup
		// when
		for range goroutines {
			wg.Go(p.Close)
		}
		wg.Wait()
		// then
		assert.False(t, p.Running())
	})
}

func TestProcessRunning(t *testing.T) {
	t.Run("returns true while the process is running", func(t *testing.T) {
		// given
		p := startProcess(t, "cat")
		// when
		result := p.Running()
		// then
		assert.True(t, result)
	})

	t.Run("returns false after the process exits on its own", func(t *testing.T) {
		// given: wait for the watcher to reap the self-exited process
		p := startProcess(t, "sh", "-c", "exit 0")
		<-p.exited
		// when
		result := p.Running()
		// then
		assert.False(t, result)
	})
}

func TestNewProcess(t *testing.T) {
	t.Run("invokes OnExit with the exit error", func(t *testing.T) {
		// given
		exited := make(chan error, 1)
		p, err := NewProcess(ProcessConfig{
			Path:   "sh",
			Args:   []string{"-c", "exit 3"},
			OnExit: func(err error) { exited <- err },
		})
		require.NoError(t, err)
		t.Cleanup(p.Close)
		// when
		result := <-exited
		// then
		var exitErr *exec.ExitError
		require.ErrorAs(t, result, &exitErr)
		expected := 3
		assert.Equal(t, expected, exitErr.ExitCode())
	})

	t.Run("stops the process when no callback is registered", func(t *testing.T) {
		// given: a process that exits on its own, with nothing to notify
		p, err := NewProcess(ProcessConfig{Path: "sh", Args: []string{"-c", "exit 0"}})
		require.NoError(t, err)
		t.Cleanup(p.Close)
		// when
		<-p.exited
		// then
		assert.False(t, p.Running())
	})
}

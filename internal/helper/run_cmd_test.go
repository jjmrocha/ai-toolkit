package helper

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	t.Run("returns the output and a zero exit code", func(t *testing.T) {
		// given
		cfg := RunConfig{Path: "sh", Args: []string{"-c", "echo first; echo second"}}
		// when
		result, err := Run(t.Context(), cfg)
		// then
		require.NoError(t, err)
		expected := RunResult{ExitCode: 0, Output: []string{"first", "second"}}
		assert.Equal(t, expected, result)
	})

	t.Run("returns the exit code of a failing command", func(t *testing.T) {
		// given
		cfg := RunConfig{Path: "sh", Args: []string{"-c", "echo bad; exit 3"}}
		// when
		result, err := Run(t.Context(), cfg)
		// then
		require.NoError(t, err)
		expected := RunResult{ExitCode: 3, Output: []string{"bad"}}
		assert.Equal(t, expected, result)
	})

	t.Run("merges stderr into the output", func(t *testing.T) {
		// given
		cfg := RunConfig{
			Path: "sh",
			Args: []string{"-c", "echo out1; echo err1 >&2; echo out2"},
		}
		// when
		result, err := Run(t.Context(), cfg)
		// then
		require.NoError(t, err)
		expected := []string{"out1", "err1", "out2"}
		assert.Equal(t, expected, result.Output)
	})

	t.Run("keeps blank lines", func(t *testing.T) {
		// given
		cfg := RunConfig{Path: "sh", Args: []string{"-c", `printf 'a\n\nb\n'`}}
		// when
		result, err := Run(t.Context(), cfg)
		// then
		require.NoError(t, err)
		expected := []string{"a", "", "b"}
		assert.Equal(t, expected, result.Output)
	})

	t.Run("returns no output for a silent command", func(t *testing.T) {
		// given
		cfg := RunConfig{Path: "sh", Args: []string{"-c", "exit 0"}}
		// when
		result, err := Run(t.Context(), cfg)
		// then
		require.NoError(t, err)
		assert.Empty(t, result.Output)
	})

	t.Run("does not wait on a command that reads stdin", func(t *testing.T) {
		// given
		cfg := RunConfig{Path: "sh", Args: []string{"-c", "cat"}}
		// when
		result, err := Run(t.Context(), cfg)
		// then
		require.NoError(t, err)
		assert.Empty(t, result.Output)
	})

	t.Run("collects everything when MaxOutputBytes is not set", func(t *testing.T) {
		// given
		cfg := RunConfig{Path: "sh", Args: []string{"-c", "echo aaaa; echo bbbb; echo cccc"}}
		// when
		result, err := Run(t.Context(), cfg)
		// then
		require.NoError(t, err)
		expected := RunResult{ExitCode: 0, Output: []string{"aaaa", "bbbb", "cccc"}}
		assert.Equal(t, expected, result)
	})

	t.Run("keeps the line that passes MaxOutputBytes and stops there", func(t *testing.T) {
		// given: each line costs its own length plus the newline
		cfg := RunConfig{
			Path:           "sh",
			Args:           []string{"-c", "echo aaaa; echo bbbb; echo cccc; echo dddd"},
			MaxOutputBytes: 10,
		}
		// when
		result, err := Run(t.Context(), cfg)
		// then
		require.NoError(t, err)
		expected := RunResult{ExitCode: 0, Output: []string{"aaaa", "bbbb", "cccc"}, Truncated: true}
		assert.Equal(t, expected, result)
	})

	t.Run("is not truncated when the output fits exactly", func(t *testing.T) {
		// given
		cfg := RunConfig{
			Path:           "sh",
			Args:           []string{"-c", "echo aaaa; echo bbbb"},
			MaxOutputBytes: 10,
		}
		// when
		result, err := Run(t.Context(), cfg)
		// then
		require.NoError(t, err)
		assert.False(t, result.Truncated)
		assert.Len(t, result.Output, 2)
	})

	t.Run("keeps the exit code of a command that finished before the cap stopped it", func(t *testing.T) {
		// given: the command is long gone by the time the cap is reached
		cfg := RunConfig{
			Path:           "sh",
			Args:           []string{"-c", "echo aaaa; echo bbbb; echo cccc; exit 2"},
			MaxOutputBytes: 5,
		}
		// when
		result, err := Run(t.Context(), cfg)
		// then
		require.NoError(t, err)
		assert.True(t, result.Truncated)
		assert.Equal(t, 2, result.ExitCode)
	})

	t.Run("stops a command that keeps running past MaxOutputBytes", func(t *testing.T) {
		// given: without a kill this would take thirty seconds
		cfg := RunConfig{
			Path:           "sh",
			Args:           []string{"-c", "echo aaaa; echo bbbb; sleep 30"},
			MaxOutputBytes: 5,
		}
		// when
		start := time.Now()
		result, err := Run(t.Context(), cfg)
		// then
		require.NoError(t, err)
		assert.True(t, result.Truncated)
		assert.Less(t, time.Since(start), 10*time.Second)
	})

	t.Run("runs the command in Dir", func(t *testing.T) {
		// given
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "data.txt"), []byte("payload"), 0o600))
		cfg := RunConfig{Path: "cat", Args: []string{"data.txt"}, Dir: dir}
		// when
		result, err := Run(t.Context(), cfg)
		// then
		require.NoError(t, err)
		expected := []string{"payload"}
		assert.Equal(t, expected, result.Output)
	})

	t.Run("returns an error when the command cannot start", func(t *testing.T) {
		// given
		cfg := RunConfig{Path: filepath.Join(t.TempDir(), "missing")}
		// when
		_, err := Run(t.Context(), cfg)
		// then
		assert.Error(t, err)
	})

	t.Run("stops when the context is done", func(t *testing.T) {
		// given
		cfg := RunConfig{Path: "sh", Args: []string{"-c", "sleep 30"}}
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		// when
		_, err := Run(ctx, cfg)
		// then
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

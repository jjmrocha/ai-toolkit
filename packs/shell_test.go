package packs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runShellTool(t *testing.T, args map[string]any) (string, error) {
	t.Helper()

	toolBox := tools.NewToolBox()
	pack := ShellTools(toolBox)

	defer func() { _ = pack.Close() }()

	call := llm.ToolCall{ID: "call-1", Name: shellToolName, Arguments: args}

	message, err := toolBox.Execute(t.Context(), call)
	if err != nil {
		return "", err
	}

	return message.Content, nil
}

func TestShellTools(t *testing.T) {
	t.Run("registers the shell tool", func(t *testing.T) {
		// given
		toolBox := tools.NewToolBox()
		// when
		pack := ShellTools(toolBox)

		defer func() { _ = pack.Close() }()
		// then
		require.Len(t, toolBox.Tools(), 1)
		assert.Equal(t, shellToolName, toolBox.Tools()[0].Name)
	})

	t.Run("removes the tool on close", func(t *testing.T) {
		// given
		toolBox := tools.NewToolBox()
		pack := ShellTools(toolBox)
		// when
		err := pack.Close()
		// then
		require.NoError(t, err)
		assert.Empty(t, toolBox.Tools())
	})

	t.Run("closes more than once without failing", func(t *testing.T) {
		// given
		toolBox := tools.NewToolBox()
		pack := ShellTools(toolBox)
		require.NoError(t, pack.Close())
		// when
		err := pack.Close()
		// then
		require.NoError(t, err)
	})
}

func TestShellRunTool(t *testing.T) {
	t.Run("returns the output and the exit status", func(t *testing.T) {
		// given
		args := map[string]any{"command": "echo hello"}
		// when
		result, err := runShellTool(t, args)
		// then
		require.NoError(t, err)
		expected := "exit status: 0\n<output>\nhello\n</output>"
		assert.Equal(t, expected, result)
	})

	t.Run("captures stdout and stderr in the order they were written", func(t *testing.T) {
		// given
		args := map[string]any{"command": "echo out1; echo err1 >&2; echo out2"}
		// when
		result, err := runShellTool(t, args)
		// then
		require.NoError(t, err)
		expected := "exit status: 0\n<output>\nout1\nerr1\nout2\n</output>"
		assert.Equal(t, expected, result)
	})

	t.Run("reports a non-zero exit without failing", func(t *testing.T) {
		// given
		args := map[string]any{"command": "echo broken >&2; exit 3"}
		// when
		result, err := runShellTool(t, args)
		// then
		require.NoError(t, err)
		expected := "exit status: 3\n<output>\nbroken\n</output>"
		assert.Equal(t, expected, result)
	})

	t.Run("runs in the requested folder", func(t *testing.T) {
		// given
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "data.txt"), []byte("payload"), 0o600))
		args := map[string]any{"command": "cat data.txt", "workdir": dir}
		// when
		result, err := runShellTool(t, args)
		// then
		require.NoError(t, err)
		expected := "exit status: 0\n<output>\npayload\n</output>"
		assert.Equal(t, expected, result)
	})

	t.Run("runs in the process folder by default", func(t *testing.T) {
		// given
		args := map[string]any{"command": "ls"}
		// when
		result, err := runShellTool(t, args)
		// then
		require.NoError(t, err)
		assert.Contains(t, result, "shell_test.go")
	})

	t.Run("reports a timeout without failing", func(t *testing.T) {
		// given
		args := map[string]any{"command": "sleep 5", "timeout_ms": float64(200)}
		// when
		result, err := runShellTool(t, args)
		// then
		require.NoError(t, err)
		expected := "timed out after 200 ms, retry with a larger timeout_ms if the command needs longer"
		assert.Equal(t, expected, result)
	})

	t.Run("truncates output past the cap", func(t *testing.T) {
		// given
		args := map[string]any{"command": `awk 'BEGIN{for(i=0;i<1600;i++) printf "%1000s\n", "x"}'`}
		// when
		result, err := runShellTool(t, args)
		// then
		require.NoError(t, err)
		assert.Contains(t, result, `<output truncated="true">`)
		assert.Less(t, len(result), maxShellOutputBytes+1024)
	})

	t.Run("fails on a folder it cannot run in", func(t *testing.T) {
		// given
		args := map[string]any{"command": "echo hello", "workdir": filepath.Join(t.TempDir(), "missing")}
		// when
		_, err := runShellTool(t, args)
		// then
		assert.Error(t, err)
	})

	t.Run("rejects a call it cannot run", func(t *testing.T) {
		testCases := []struct {
			name     string
			args     map[string]any
			expected error
		}{
			{
				name:     "without a command",
				args:     map[string]any{},
				expected: tools.ErrFieldNotFound,
			},
			{
				name:     "with a timeout over the ceiling",
				args:     map[string]any{"command": "echo hello", "timeout_ms": float64(600001)},
				expected: ErrInvalidTimeout,
			},
			{
				name:     "with a timeout of zero",
				args:     map[string]any{"command": "echo hello", "timeout_ms": float64(0)},
				expected: ErrInvalidTimeout,
			},
			{
				name:     "with a negative timeout",
				args:     map[string]any{"command": "echo hello", "timeout_ms": float64(-1)},
				expected: ErrInvalidTimeout,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// when
				_, err := runShellTool(t, tc.args)
				// then
				assert.ErrorIs(t, err, tc.expected)
			})
		}
	})
}

package packs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rootWith(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()

	for name, content := range files {
		full := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o700))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o600))
	}

	return dir
}

func runFileTool(t *testing.T, root string, name string, args map[string]any) (string, error) {
	t.Helper()

	toolBox := tools.NewToolBox()

	pack, err := FileTools(toolBox, root)
	require.NoError(t, err)

	defer func() { _ = pack.Close() }()

	call := llm.ToolCall{ID: "call-1", Name: name, Arguments: args}

	message, err := toolBox.Execute(t.Context(), call)
	if err != nil {
		return "", err
	}

	return message.Content, nil
}

func TestFileTools(t *testing.T) {
	t.Run("registers the file tools", func(t *testing.T) {
		// given
		toolBox := tools.NewToolBox()
		// when
		pack, err := FileTools(toolBox, t.TempDir())
		require.NoError(t, err)

		defer func() { _ = pack.Close() }()
		// then
		var names []string
		for _, tool := range toolBox.Tools() {
			names = append(names, tool.Name)
		}

		expected := []string{deleteToolName, editToolName, listToolName, readToolName, workdirToolName, writeToolName}
		assert.Equal(t, expected, names)
	})

	t.Run("removes the tools on close", func(t *testing.T) {
		// given
		toolBox := tools.NewToolBox()
		pack, err := FileTools(toolBox, t.TempDir())
		require.NoError(t, err)
		// when
		err = pack.Close()
		// then
		require.NoError(t, err)
		assert.Empty(t, toolBox.Tools())
	})

	t.Run("closes more than once without failing", func(t *testing.T) {
		// given
		toolBox := tools.NewToolBox()
		pack, err := FileTools(toolBox, t.TempDir())
		require.NoError(t, err)
		require.NoError(t, pack.Close())
		// when
		err = pack.Close()
		// then
		require.NoError(t, err)
	})

	t.Run("rejects a root it cannot open", func(t *testing.T) {
		// given
		root := filepath.Join(t.TempDir(), "missing")
		toolBox := tools.NewToolBox()
		// when
		pack, err := FileTools(toolBox, root)
		// then
		require.Error(t, err)
		assert.Nil(t, pack)
		assert.Empty(t, toolBox.Tools())
	})
}

func TestFileWorkdirTool(t *testing.T) {
	t.Run("returns the absolute path of the folder the tools are confined to", func(t *testing.T) {
		// given
		root := rootWith(t, nil)
		// when
		result, err := runFileTool(t, root, workdirToolName, map[string]any{})
		// then
		require.NoError(t, err)
		assert.Equal(t, root, result)
	})

	t.Run("returns an absolute path for a pack rooted at a relative one", func(t *testing.T) {
		// given
		root := rootWith(t, nil)
		t.Chdir(root)
		// when
		result, err := runFileTool(t, ".", workdirToolName, map[string]any{})
		// then
		require.NoError(t, err)
		assert.True(t, filepath.IsAbs(result))

		wanted, err := os.Stat(root)
		require.NoError(t, err)

		got, err := os.Stat(result)
		require.NoError(t, err)
		assert.True(t, os.SameFile(wanted, got))
	})
}

func TestFileReadTool(t *testing.T) {
	t.Run("returns the file with the range it read", func(t *testing.T) {
		// given
		root := rootWith(t, map[string]string{"notes.md": "one\ntwo\nthree\n"})
		// when
		result, err := runFileTool(t, root, readToolName, map[string]any{"path": "notes.md"})
		// then
		require.NoError(t, err)
		expected := "<file lines=\"1-3 of 3\">\none\ntwo\nthree\n</file>"
		assert.Equal(t, expected, result)
	})

	t.Run("reads a file in a subfolder", func(t *testing.T) {
		// given
		root := rootWith(t, map[string]string{"reports/q1.md": "payload\n"})
		// when
		result, err := runFileTool(t, root, readToolName, map[string]any{"path": "reports/q1.md"})
		// then
		require.NoError(t, err)
		expected := "<file lines=\"1-1 of 1\">\npayload\n</file>"
		assert.Equal(t, expected, result)
	})

	t.Run("reads the page the call asks for", func(t *testing.T) {
		// given
		root := rootWith(t, map[string]string{"notes.md": "one\ntwo\nthree\nfour\nfive\n"})
		args := map[string]any{"path": "notes.md", "offset": float64(2), "limit": float64(2)}
		// when
		result, err := runFileTool(t, root, readToolName, args)
		// then
		require.NoError(t, err)
		expected := "<file lines=\"2-3 of 5\">\ntwo\nthree\n</file>"
		assert.Equal(t, expected, result)
	})

	t.Run("stops at the byte cap", func(t *testing.T) {
		// given
		line := strings.Repeat("x", 999) + "\n"
		root := rootWith(t, map[string]string{"big.txt": strings.Repeat(line, 2000)})
		args := map[string]any{"path": "big.txt", "limit": float64(2000)}
		// when
		result, err := runFileTool(t, root, readToolName, args)
		// then
		require.NoError(t, err)
		assert.Contains(t, result, ` of 2000">`)
		assert.NotContains(t, result, `"1-2000 of 2000"`)
		assert.Less(t, len(result), maxFileReadBytes+1024)
	})

	t.Run("rejects a call it cannot read", func(t *testing.T) {
		testCases := []struct {
			name     string
			args     map[string]any
			expected error
		}{
			{
				name:     "without a path",
				args:     map[string]any{},
				expected: tools.ErrFieldNotFound,
			},
			{
				name:     "with an offset below one",
				args:     map[string]any{"path": "notes.md", "offset": float64(0)},
				expected: ErrInvalidRange,
			},
			{
				name:     "with a limit below one",
				args:     map[string]any{"path": "notes.md", "limit": float64(0)},
				expected: ErrInvalidRange,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// given
				root := rootWith(t, map[string]string{"notes.md": "one\n"})
				// when
				_, err := runFileTool(t, root, readToolName, tc.args)
				// then
				assert.ErrorIs(t, err, tc.expected)
			})
		}
	})

	t.Run("fails on a file it cannot reach", func(t *testing.T) {
		testCases := []struct {
			name string
			path string
		}{
			{name: "outside the root", path: "../outside.txt"},
			{name: "absolute", path: "/etc/hosts"},
			{name: "missing", path: "missing.md"},
			{name: "a folder", path: "reports"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// given
				root := rootWith(t, map[string]string{"reports/q1.md": "payload\n"})
				// when
				_, err := runFileTool(t, root, readToolName, map[string]any{"path": tc.path})
				// then
				require.Error(t, err)
				assert.NotContains(t, err.Error(), root)
			})
		}
	})
}

func TestFileWriteTool(t *testing.T) {
	t.Run("writes a new file", func(t *testing.T) {
		// given
		root := rootWith(t, nil)
		args := map[string]any{"path": "notes.md", "content": "one\ntwo\n"}
		// when
		result, err := runFileTool(t, root, writeToolName, args)
		// then
		require.NoError(t, err)
		assert.Equal(t, "wrote 8 bytes to notes.md - "+filepath.Join(root, "notes.md"), result)
		written, err := os.ReadFile(filepath.Join(root, "notes.md"))
		require.NoError(t, err)
		assert.Equal(t, "one\ntwo\n", string(written))
	})

	t.Run("overwrites an existing file", func(t *testing.T) {
		// given
		root := rootWith(t, map[string]string{"notes.md": "old content\n"})
		args := map[string]any{"path": "notes.md", "content": "new\n"}
		// when
		_, err := runFileTool(t, root, writeToolName, args)
		// then
		require.NoError(t, err)
		written, err := os.ReadFile(filepath.Join(root, "notes.md"))
		require.NoError(t, err)
		assert.Equal(t, "new\n", string(written))
	})

	t.Run("creates the folders the path needs", func(t *testing.T) {
		// given
		root := rootWith(t, nil)
		args := map[string]any{"path": "reports/2026/q1.md", "content": "payload\n"}
		// when
		result, err := runFileTool(t, root, writeToolName, args)
		// then
		require.NoError(t, err)
		assert.Equal(t, "wrote 8 bytes to reports/2026/q1.md - "+filepath.Join(root, "reports/2026/q1.md"), result)
		written, err := os.ReadFile(filepath.Join(root, "reports/2026/q1.md"))
		require.NoError(t, err)
		assert.Equal(t, "payload\n", string(written))
	})

	t.Run("rejects a call it cannot write", func(t *testing.T) {
		testCases := []struct {
			name     string
			args     map[string]any
			expected error
		}{
			{
				name:     "without a path",
				args:     map[string]any{"content": "payload"},
				expected: tools.ErrFieldNotFound,
			},
			{
				name:     "without content",
				args:     map[string]any{"path": "notes.md"},
				expected: tools.ErrFieldNotFound,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// given
				root := rootWith(t, nil)
				// when
				_, err := runFileTool(t, root, writeToolName, tc.args)
				// then
				assert.ErrorIs(t, err, tc.expected)
			})
		}
	})

	t.Run("fails on a path it cannot reach", func(t *testing.T) {
		testCases := []struct {
			name string
			path string
		}{
			{name: "outside the root", path: "../outside.txt"},
			{name: "absolute", path: "/tmp/outside.txt"},
			{name: "a folder", path: "reports"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// given
				root := rootWith(t, map[string]string{"reports/q1.md": "payload\n"})
				args := map[string]any{"path": tc.path, "content": "payload"}
				// when
				_, err := runFileTool(t, root, writeToolName, args)
				// then
				require.Error(t, err)
				assert.NotContains(t, err.Error(), root)
			})
		}
	})
}

func TestFileEditTool(t *testing.T) {
	t.Run("replaces the text it was given", func(t *testing.T) {
		// given
		root := rootWith(t, map[string]string{"notes.md": "one\ntwo\nthree\n"})
		args := map[string]any{"path": "notes.md", "old_string": "two", "new_string": "four"}
		// when
		result, err := runFileTool(t, root, editToolName, args)
		// then
		require.NoError(t, err)
		assert.Equal(t, "edited notes.md", result)
		written, err := os.ReadFile(filepath.Join(root, "notes.md"))
		require.NoError(t, err)
		assert.Equal(t, "one\nfour\nthree\n", string(written))
	})

	t.Run("removes the text when the replacement is empty", func(t *testing.T) {
		// given
		root := rootWith(t, map[string]string{"notes.md": "one\ntwo\n"})
		args := map[string]any{"path": "notes.md", "old_string": "two\n", "new_string": ""}
		// when
		_, err := runFileTool(t, root, editToolName, args)
		// then
		require.NoError(t, err)
		written, err := os.ReadFile(filepath.Join(root, "notes.md"))
		require.NoError(t, err)
		assert.Equal(t, "one\n", string(written))
	})

	t.Run("leaves the file alone when it cannot place the edit", func(t *testing.T) {
		testCases := []struct {
			name     string
			old      string
			expected error
		}{
			{name: "no match", old: "missing", expected: ErrNoMatch},
			{name: "several matches", old: "one", expected: ErrManyMatches},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// given
				content := "one\ntwo\none\n"
				root := rootWith(t, map[string]string{"notes.md": content})
				args := map[string]any{"path": "notes.md", "old_string": tc.old, "new_string": "four"}
				// when
				_, err := runFileTool(t, root, editToolName, args)
				// then
				assert.ErrorIs(t, err, tc.expected)
				written, readErr := os.ReadFile(filepath.Join(root, "notes.md"))
				require.NoError(t, readErr)
				assert.Equal(t, content, string(written))
			})
		}
	})

	t.Run("rejects a call it cannot edit", func(t *testing.T) {
		testCases := []struct {
			name     string
			args     map[string]any
			expected error
		}{
			{
				name:     "without a path",
				args:     map[string]any{"old_string": "one", "new_string": "two"},
				expected: tools.ErrFieldNotFound,
			},
			{
				name:     "without the text to replace",
				args:     map[string]any{"path": "notes.md", "new_string": "two"},
				expected: tools.ErrFieldNotFound,
			},
			{
				name:     "without the replacement",
				args:     map[string]any{"path": "notes.md", "old_string": "one"},
				expected: tools.ErrFieldNotFound,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// given
				root := rootWith(t, map[string]string{"notes.md": "one\n"})
				// when
				_, err := runFileTool(t, root, editToolName, tc.args)
				// then
				assert.ErrorIs(t, err, tc.expected)
			})
		}
	})

	t.Run("fails on a file it cannot reach", func(t *testing.T) {
		// given
		root := rootWith(t, map[string]string{"notes.md": "one\n"})
		args := map[string]any{"path": "../outside.txt", "old_string": "one", "new_string": "two"}
		// when
		_, err := runFileTool(t, root, editToolName, args)
		// then
		require.Error(t, err)
		assert.NotContains(t, err.Error(), root)
	})
}

func TestFileListTool(t *testing.T) {
	t.Run("lists the root by default", func(t *testing.T) {
		// given
		root := rootWith(t, map[string]string{"notes.md": "one\ntwo\n", "reports/q1.md": "payload\n"})
		// when
		result, err := runFileTool(t, root, listToolName, map[string]any{})
		// then
		require.NoError(t, err)
		expected := "<dir path=\".\">\n" +
			"<file name=\"notes.md\" size=\"8\" path=\"" + filepath.Join(root, "notes.md") + "\"/>\n" +
			"<dir name=\"reports\" path=\"" + filepath.Join(root, "reports") + "\"/>\n" +
			"</dir>"
		assert.Equal(t, expected, result)
	})

	t.Run("lists the folder the call asks for", func(t *testing.T) {
		// given
		root := rootWith(t, map[string]string{"reports/q1.md": "payload\n"})
		// when
		result, err := runFileTool(t, root, listToolName, map[string]any{"path": "reports"})
		// then
		require.NoError(t, err)
		expected := "<dir path=\"reports\">\n" +
			"<file name=\"q1.md\" size=\"8\" path=\"" + filepath.Join(root, "reports/q1.md") + "\"/>\n" +
			"</dir>"
		assert.Equal(t, expected, result)
	})

	t.Run("lists an empty folder", func(t *testing.T) {
		// given
		root := rootWith(t, nil)
		// when
		result, err := runFileTool(t, root, listToolName, map[string]any{})
		// then
		require.NoError(t, err)
		expected := "<dir path=\".\">\n</dir>"
		assert.Equal(t, expected, result)
	})

	t.Run("fails on a folder it cannot reach", func(t *testing.T) {
		testCases := []struct {
			name string
			path string
		}{
			{name: "outside the root", path: "../"},
			{name: "absolute", path: "/etc"},
			{name: "missing", path: "missing"},
			{name: "a file", path: "notes.md"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// given
				root := rootWith(t, map[string]string{"notes.md": "one\n"})
				// when
				_, err := runFileTool(t, root, listToolName, map[string]any{"path": tc.path})
				// then
				require.Error(t, err)
				assert.NotContains(t, err.Error(), root)
			})
		}
	})
}

func TestFileDeleteTool(t *testing.T) {
	t.Run("deletes a file", func(t *testing.T) {
		// given
		root := rootWith(t, map[string]string{"notes.md": "one\n"})
		// when
		result, err := runFileTool(t, root, deleteToolName, map[string]any{"path": "notes.md"})
		// then
		require.NoError(t, err)
		assert.Equal(t, "deleted notes.md", result)
		assert.NoFileExists(t, filepath.Join(root, "notes.md"))
	})

	t.Run("deletes an empty folder", func(t *testing.T) {
		// given
		root := rootWith(t, nil)
		require.NoError(t, os.Mkdir(filepath.Join(root, "reports"), 0o700))
		// when
		_, err := runFileTool(t, root, deleteToolName, map[string]any{"path": "reports"})
		// then
		require.NoError(t, err)
		assert.NoDirExists(t, filepath.Join(root, "reports"))
	})

	t.Run("keeps a folder that still holds something", func(t *testing.T) {
		// given
		root := rootWith(t, map[string]string{"reports/q1.md": "payload\n"})
		// when
		_, err := runFileTool(t, root, deleteToolName, map[string]any{"path": "reports"})
		// then
		require.Error(t, err)
		assert.FileExists(t, filepath.Join(root, "reports/q1.md"))
	})

	t.Run("rejects a call without a path", func(t *testing.T) {
		// given
		root := rootWith(t, nil)
		// when
		_, err := runFileTool(t, root, deleteToolName, map[string]any{})
		// then
		assert.ErrorIs(t, err, tools.ErrFieldNotFound)
	})

	t.Run("fails on a path it cannot reach", func(t *testing.T) {
		testCases := []struct {
			name string
			path string
		}{
			{name: "outside the root", path: "../outside.txt"},
			{name: "absolute", path: "/etc/hosts"},
			{name: "missing", path: "missing.md"},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// given
				root := rootWith(t, map[string]string{"notes.md": "one\n"})
				// when
				_, err := runFileTool(t, root, deleteToolName, map[string]any{"path": tc.path})
				// then
				require.Error(t, err)
				assert.NotContains(t, err.Error(), root)
			})
		}
	})
}

package skills

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeSkillWithFiles(t *testing.T, content string, files map[string]string) string {
	t.Helper()

	path := writeSkill(t, content)

	for name, data := range files {
		full := filepath.Join(path, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o700))
		require.NoError(t, os.WriteFile(full, []byte(data), 0o600))
	}

	return path
}

func writeExecutable(t *testing.T, path string, name string, script string) {
	t.Helper()

	full := filepath.Join(path, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o700))
	require.NoError(t, os.WriteFile(full, []byte(script), 0o700))
}

func collectionWith(t *testing.T, paths ...string) *Collection {
	t.Helper()

	collection := NewCollection()
	for _, path := range paths {
		require.NoError(t, collection.Add(path))
	}

	return collection
}

func executeTool(t *testing.T, collection *Collection, name string, args map[string]any) (string, error) {
	t.Helper()

	return executeToolContext(t, t.Context(), collection, name, args)
}

func executeToolContext(t *testing.T, ctx context.Context, collection *Collection, name string, args map[string]any) (string, error) {
	t.Helper()

	toolBox := tools.NewToolBox()
	collection.RegisterTools(toolBox)

	message, err := toolBox.Execute(ctx, llm.ToolCall{ID: "c1", Name: name, Arguments: args})
	if err != nil {
		return "", err
	}

	return message.Content, nil
}

func toolNames(toolBox *tools.ToolBox) []string {
	names := make([]string, 0)
	for _, tool := range toolBox.Tools() {
		names = append(names, tool.Name)
	}

	return names
}

func TestCollectionRegisterTools(t *testing.T) {
	t.Run("registers the load, load-file and execute-file tools", func(t *testing.T) {
		// given
		collection := collectionWith(t, writeSkill(t, validSkill))
		toolBox := tools.NewToolBox()
		// when
		collection.RegisterTools(toolBox)
		// then
		result := toolNames(toolBox)
		expected := []string{loadToolName, loadFileToolName, executeFileToolName}
		assert.ElementsMatch(t, expected, result)
	})

	t.Run("leaves the caller's own tools untouched", func(t *testing.T) {
		// given
		collection := collectionWith(t, writeSkill(t, validSkill))
		toolBox := tools.NewToolBox()
		require.NoError(t, toolBox.Add(llm.Tool{Name: "echo"}, func(context.Context, map[string]any) (string, error) { return "ok", nil }))
		// when
		collection.RegisterTools(toolBox)
		// then
		assert.Contains(t, toolNames(toolBox), "echo")
	})
}

func TestCollectionUnregisterTools(t *testing.T) {
	t.Run("removes the load, load-file and execute-file tools", func(t *testing.T) {
		// given
		collection := collectionWith(t, writeSkill(t, validSkill))
		toolBox := tools.NewToolBox()
		collection.RegisterTools(toolBox)
		// when
		collection.UnregisterTools(toolBox)
		// then
		assert.Empty(t, toolNames(toolBox))
	})

	t.Run("leaves the caller's own tools untouched", func(t *testing.T) {
		// given
		collection := collectionWith(t, writeSkill(t, validSkill))
		toolBox := tools.NewToolBox()
		require.NoError(t, toolBox.Add(llm.Tool{Name: "echo"}, func(context.Context, map[string]any) (string, error) { return "ok", nil }))
		collection.RegisterTools(toolBox)
		// when
		collection.UnregisterTools(toolBox)
		// then
		assert.Equal(t, []string{"echo"}, toolNames(toolBox))
	})
}

func TestLoadTool(t *testing.T) {
	t.Run("returns the skill body and its files", func(t *testing.T) {
		// given
		path := writeSkillWithFiles(t, validSkill, map[string]string{
			"references/notes.md": "notes",
			"scripts/check.sh":    "#!/bin/sh\n",
		})
		collection := collectionWith(t, path)
		// when
		result, err := executeTool(t, collection, loadToolName, map[string]any{"skill_name": "git-release"})
		// then
		require.NoError(t, err)
		expected := "<skill name=\"git-release\">\n" +
			"Do the thing.\n" +
			"\n" +
			"Files available via skill_load_file:\n" +
			"  references/notes.md\n" +
			"  scripts/check.sh\n" +
			"</skill>"
		assert.Equal(t, expected, result)
	})

	t.Run("omits the file section when the skill has no files", func(t *testing.T) {
		// given
		collection := collectionWith(t, writeSkill(t, validSkill))
		// when
		result, err := executeTool(t, collection, loadToolName, map[string]any{"skill_name": "git-release"})
		// then
		require.NoError(t, err)
		expected := "<skill name=\"git-release\">\n" +
			"Do the thing.\n" +
			"</skill>"
		assert.Equal(t, expected, result)
	})

	t.Run("names the available skills when the skill is unknown", func(t *testing.T) {
		// given
		collection := collectionWith(t, writeSkill(t, validSkill))
		// when
		_, err := executeTool(t, collection, loadToolName, map[string]any{"skill_name": "nope"})
		// then
		require.ErrorIs(t, err, ErrSkillNotFound)
		assert.ErrorContains(t, err, "git-release")
	})

	t.Run("fails when the skill_name argument is missing", func(t *testing.T) {
		// given
		collection := collectionWith(t, writeSkill(t, validSkill))
		// when
		_, err := executeTool(t, collection, loadToolName, map[string]any{})
		// then
		assert.ErrorIs(t, err, tools.ErrFieldNotFound)
	})
}

func TestLoadFileTool(t *testing.T) {
	t.Run("returns the file contents", func(t *testing.T) {
		// given
		path := writeSkillWithFiles(t, validSkill, map[string]string{"references/notes.md": "the notes"})
		collection := collectionWith(t, path)
		// when
		result, err := executeTool(t, collection, loadFileToolName, map[string]any{
			"skill_name": "git-release",
			"path":       "references/notes.md",
		})
		// then
		require.NoError(t, err)
		assert.Equal(t, "the notes", result)
	})

	t.Run("lists the skill's files when the path is unknown", func(t *testing.T) {
		// given
		path := writeSkillWithFiles(t, validSkill, map[string]string{"references/notes.md": "the notes"})
		collection := collectionWith(t, path)
		// when
		_, err := executeTool(t, collection, loadFileToolName, map[string]any{
			"skill_name": "git-release",
			"path":       "references/missing.md",
		})
		// then
		require.ErrorIs(t, err, ErrFileNotFound)
		assert.ErrorContains(t, err, "references/notes.md")
	})

	t.Run("names the available skills when the skill is unknown", func(t *testing.T) {
		// given
		collection := collectionWith(t, writeSkill(t, validSkill))
		// when
		_, err := executeTool(t, collection, loadFileToolName, map[string]any{
			"skill_name": "nope",
			"path":       "whatever.md",
		})
		// then
		assert.ErrorIs(t, err, ErrSkillNotFound)
	})

	t.Run("reads a relative symlink pointing inside the skill folder", func(t *testing.T) {
		// given
		path := writeSkillWithFiles(t, validSkill, map[string]string{"references/notes.md": "the notes"})
		require.NoError(t, os.Symlink("references/notes.md", filepath.Join(path, "shortcut.md")))
		collection := collectionWith(t, path)
		// when
		listing, listErr := executeTool(t, collection, loadToolName, map[string]any{"skill_name": "git-release"})
		result, readErr := executeTool(t, collection, loadFileToolName, map[string]any{
			"skill_name": "git-release",
			"path":       "shortcut.md",
		})
		// then
		require.NoError(t, listErr)
		require.NoError(t, readErr)
		assert.Contains(t, listing, "shortcut.md")
		assert.Equal(t, "the notes", result)
	})

	t.Run("never names the skill folder when it is gone", func(t *testing.T) {
		// given
		path := writeSkillWithFiles(t, validSkill, map[string]string{"notes.md": "the notes"})
		collection := collectionWith(t, path)
		require.NoError(t, os.RemoveAll(path))
		// when
		_, err := executeTool(t, collection, loadFileToolName, map[string]any{
			"skill_name": validSkillName,
			"path":       "notes.md",
		})
		// then
		require.Error(t, err)
		assert.ErrorContains(t, err, "no such file or directory")
		assert.NotContains(t, err.Error(), path)
	})

	t.Run("does not expose a symlink pointing outside the skill folder", func(t *testing.T) {
		// given
		secret := filepath.Join(t.TempDir(), "secret.txt")
		require.NoError(t, os.WriteFile(secret, []byte("classified"), 0o600))
		path := writeSkill(t, validSkill)
		require.NoError(t, os.Symlink(secret, filepath.Join(path, "escape.txt")))
		collection := collectionWith(t, path)
		// when
		listing, listErr := executeTool(t, collection, loadToolName, map[string]any{"skill_name": "git-release"})
		_, readErr := executeTool(t, collection, loadFileToolName, map[string]any{
			"skill_name": "git-release",
			"path":       "escape.txt",
		})
		// then
		require.NoError(t, listErr)
		assert.NotContains(t, listing, "escape.txt")
		assert.ErrorIs(t, readErr, ErrFileNotFound)
	})
}

func TestExecuteFileTool(t *testing.T) {
	t.Run("returns the output and the exit status", func(t *testing.T) {
		// given
		path := writeSkill(t, validSkill)
		writeExecutable(t, path, "scripts/hello.sh", "#!/bin/sh\necho hello\n")
		collection := collectionWith(t, path)
		// when
		result, err := executeTool(t, collection, executeFileToolName, map[string]any{
			"skill_name": "git-release",
			"path":       "scripts/hello.sh",
		})
		// then
		require.NoError(t, err)
		expected := "exit status: 0\n<output>\nhello\n</output>"
		assert.Equal(t, expected, result)
	})

	t.Run("passes the arguments in order", func(t *testing.T) {
		// given
		path := writeSkill(t, validSkill)
		writeExecutable(t, path, "scripts/echo.sh", "#!/bin/sh\nfor a in \"$@\"; do echo \"[$a]\"; done\n")
		collection := collectionWith(t, path)
		// when
		result, err := executeTool(t, collection, executeFileToolName, map[string]any{
			"skill_name": "git-release",
			"path":       "scripts/echo.sh",
			"args":       []any{"one", "two three", "four"},
		})
		// then
		require.NoError(t, err)
		expected := "exit status: 0\n<output>\n[one]\n[two three]\n[four]\n</output>"
		assert.Equal(t, expected, result)
	})

	t.Run("captures stdout and stderr in the order they were written", func(t *testing.T) {
		// given
		path := writeSkill(t, validSkill)
		writeExecutable(t, path, "run.sh", "#!/bin/sh\necho out1\necho err1 >&2\necho out2\n")
		collection := collectionWith(t, path)
		// when
		result, err := executeTool(t, collection, executeFileToolName, map[string]any{
			"skill_name": "git-release",
			"path":       "run.sh",
		})
		// then
		require.NoError(t, err)
		expected := "exit status: 0\n<output>\nout1\nerr1\nout2\n</output>"
		assert.Equal(t, expected, result)
	})

	t.Run("runs in the skill folder", func(t *testing.T) {
		// given
		path := writeSkillWithFiles(t, validSkill, map[string]string{"data.txt": "payload"})
		writeExecutable(t, path, "run.sh", "#!/bin/sh\ncat data.txt\n")
		collection := collectionWith(t, path)
		// when
		result, err := executeTool(t, collection, executeFileToolName, map[string]any{
			"skill_name": "git-release",
			"path":       "run.sh",
		})
		// then
		require.NoError(t, err)
		assert.Contains(t, result, "payload")
	})

	t.Run("reports a non-zero exit without failing", func(t *testing.T) {
		// given
		path := writeSkill(t, validSkill)
		writeExecutable(t, path, "run.sh", "#!/bin/sh\necho broken >&2\nexit 3\n")
		collection := collectionWith(t, path)
		// when
		result, err := executeTool(t, collection, executeFileToolName, map[string]any{
			"skill_name": "git-release",
			"path":       "run.sh",
		})
		// then
		require.NoError(t, err)
		expected := "exit status: 3\n<output>\nbroken\n</output>"
		assert.Equal(t, expected, result)
	})

	t.Run("truncates output past the cap", func(t *testing.T) {
		// given
		path := writeSkill(t, validSkill)
		writeExecutable(t, path, "flood.sh", "#!/bin/sh\nawk 'BEGIN{for(i=0;i<1600;i++) printf \"%1000s\\n\", \"x\"}'\n")
		collection := collectionWith(t, path)
		// when
		result, err := executeTool(t, collection, executeFileToolName, map[string]any{
			"skill_name": "git-release",
			"path":       "flood.sh",
		})
		// then
		require.NoError(t, err)
		assert.Contains(t, result, `<output truncated="true">`)
		assert.Less(t, len(result), maxOutputBytes+1024)
	})

	t.Run("rejects a file it may not run", func(t *testing.T) {
		testCases := []struct {
			name     string
			setup    func(t *testing.T) (*Collection, string)
			file     string
			expected error
		}{
			{
				name: "unknown skill",
				setup: func(t *testing.T) (*Collection, string) {
					return collectionWith(t, writeSkill(t, validSkill)), "nope"
				},
				file:     "run.sh",
				expected: ErrSkillNotFound,
			},
			{
				name: "path outside the skill's file list",
				setup: func(t *testing.T) (*Collection, string) {
					path := writeSkill(t, validSkill)
					writeExecutable(t, path, "run.sh", "#!/bin/sh\necho hello\n")

					return collectionWith(t, path), validSkillName
				},
				file:     "scripts/other.sh",
				expected: ErrFileNotFound,
			},
			{
				name: "path escaping the skill folder",
				setup: func(t *testing.T) (*Collection, string) {
					return collectionWith(t, writeSkill(t, validSkill)), validSkillName
				},
				file:     "../../../bin/sh",
				expected: ErrFileNotFound,
			},
			{
				name: "symlink pointing outside the skill folder",
				setup: func(t *testing.T) (*Collection, string) {
					target := filepath.Join(t.TempDir(), "outside.sh")
					require.NoError(t, os.WriteFile(target, []byte("#!/bin/sh\necho leaked\n"), 0o700))
					path := writeSkill(t, validSkill)
					require.NoError(t, os.Symlink(target, filepath.Join(path, "escape.sh")))

					return collectionWith(t, path), validSkillName
				},
				file:     "escape.sh",
				expected: ErrFileNotFound,
			},
		}

		for _, testCase := range testCases {
			t.Run(testCase.name, func(t *testing.T) {
				// given
				collection, skillName := testCase.setup(t)
				// when
				_, err := executeTool(t, collection, executeFileToolName, map[string]any{
					"skill_name": skillName,
					"path":       testCase.file,
				})
				// then
				assert.ErrorIs(t, err, testCase.expected)
			})
		}
	})

	t.Run("fails to run a file without the execute bit", func(t *testing.T) {
		// given
		path := writeSkillWithFiles(t, validSkill, map[string]string{"run.sh": "#!/bin/sh\necho hello\n"})
		collection := collectionWith(t, path)
		// when
		_, err := executeTool(t, collection, executeFileToolName, map[string]any{
			"skill_name": "git-release",
			"path":       "run.sh",
		})
		// then
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrFileNotFound)
		assert.ErrorContains(t, err, "permission denied")
		assert.NotContains(t, err.Error(), path)
	})

	t.Run("stops when the context is done", func(t *testing.T) {
		// given
		path := writeSkill(t, validSkill)
		writeExecutable(t, path, "sleep.sh", "#!/bin/sh\nsleep 30\n")
		collection := collectionWith(t, path)
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		// when
		_, err := executeToolContext(t, ctx, collection, executeFileToolName, map[string]any{
			"skill_name": "git-release",
			"path":       "sleep.sh",
		})
		// then
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("runs a skill added under a relative path", func(t *testing.T) {
		// given
		base := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(base, "myskill"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(base, "myskill", skillFile), []byte(validSkill), 0o600))
		writeExecutable(t, filepath.Join(base, "myskill"), "run.sh", "#!/bin/sh\necho hello\n")
		t.Chdir(base)
		collection := NewCollection()
		require.NoError(t, collection.Add("myskill"))
		// when
		result, err := executeTool(t, collection, executeFileToolName, map[string]any{
			"skill_name": validSkillName,
			"path":       "run.sh",
		})
		// then
		require.NoError(t, err)
		assert.Equal(t, "exit status: 0\n<output>\nhello\n</output>", result)
	})

	t.Run("runs a skill added under a relative path after the process moves", func(t *testing.T) {
		// given
		base := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(base, "myskill"), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(base, "myskill", skillFile), []byte(validSkill), 0o600))
		writeExecutable(t, filepath.Join(base, "myskill"), "run.sh", "#!/bin/sh\necho hello\n")
		t.Chdir(base)
		collection := NewCollection()
		require.NoError(t, collection.Add("myskill"))
		t.Chdir(t.TempDir())
		// when
		result, err := executeTool(t, collection, executeFileToolName, map[string]any{
			"skill_name": validSkillName,
			"path":       "run.sh",
		})
		// then
		require.NoError(t, err)
		assert.Equal(t, "exit status: 0\n<output>\nhello\n</output>", result)
	})

	t.Run("never names the skill folder", func(t *testing.T) {
		// given
		path := writeSkill(t, validSkill)
		writeExecutable(t, path, "run.sh", "#!/bin/sh\necho hello\n")
		collection := collectionWith(t, path)
		// when
		result, err := executeTool(t, collection, executeFileToolName, map[string]any{
			"skill_name": "git-release",
			"path":       "run.sh",
		})
		// then
		require.NoError(t, err)
		assert.NotContains(t, result, path)
	})
}

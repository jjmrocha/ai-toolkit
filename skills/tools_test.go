package skills

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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

	toolBox := tools.NewToolBox()
	collection.RegisterTools(toolBox)

	message, err := toolBox.Execute(t.Context(), llm.ToolCall{ID: "c1", Name: name, Arguments: args})
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
	t.Run("registers the load and load-file tools", func(t *testing.T) {
		// given
		collection := collectionWith(t, writeSkill(t, validSkill))
		toolBox := tools.NewToolBox()
		// when
		collection.RegisterTools(toolBox)
		// then
		result := toolNames(toolBox)
		expected := []string{LoadToolName, LoadFileToolName}
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
	t.Run("removes the load and load-file tools", func(t *testing.T) {
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
		result, err := executeTool(t, collection, LoadToolName, map[string]any{"skill_name": "git-release"})
		// then
		require.NoError(t, err)
		expected := "<skill name=\"git-release\">\n" +
			"# Skill: git-release\n" +
			"\n" +
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
		result, err := executeTool(t, collection, LoadToolName, map[string]any{"skill_name": "git-release"})
		// then
		require.NoError(t, err)
		expected := "<skill name=\"git-release\">\n" +
			"# Skill: git-release\n" +
			"\n" +
			"Do the thing.\n" +
			"</skill>"
		assert.Equal(t, expected, result)
	})

	t.Run("names the available skills when the skill is unknown", func(t *testing.T) {
		// given
		collection := collectionWith(t, writeSkill(t, validSkill))
		// when
		_, err := executeTool(t, collection, LoadToolName, map[string]any{"skill_name": "nope"})
		// then
		require.ErrorIs(t, err, ErrSkillNotFound)
		assert.ErrorContains(t, err, "git-release")
	})

	t.Run("fails when the skill_name argument is missing", func(t *testing.T) {
		// given
		collection := collectionWith(t, writeSkill(t, validSkill))
		// when
		_, err := executeTool(t, collection, LoadToolName, map[string]any{})
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
		result, err := executeTool(t, collection, LoadFileToolName, map[string]any{
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
		_, err := executeTool(t, collection, LoadFileToolName, map[string]any{
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
		_, err := executeTool(t, collection, LoadFileToolName, map[string]any{
			"skill_name": "nope",
			"path":       "whatever.md",
		})
		// then
		assert.ErrorIs(t, err, ErrSkillNotFound)
	})

	t.Run("does not expose a symlink pointing outside the skill folder", func(t *testing.T) {
		// given
		secret := filepath.Join(t.TempDir(), "secret.txt")
		require.NoError(t, os.WriteFile(secret, []byte("classified"), 0o600))
		path := writeSkill(t, validSkill)
		require.NoError(t, os.Symlink(secret, filepath.Join(path, "escape.txt")))
		collection := collectionWith(t, path)
		// when
		listing, listErr := executeTool(t, collection, LoadToolName, map[string]any{"skill_name": "git-release"})
		_, readErr := executeTool(t, collection, LoadFileToolName, map[string]any{
			"skill_name": "git-release",
			"path":       "escape.txt",
		})
		// then
		require.NoError(t, listErr)
		assert.NotContains(t, listing, "escape.txt")
		assert.ErrorIs(t, readErr, ErrFileNotFound)
	})
}

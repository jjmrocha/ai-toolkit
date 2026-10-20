package packs

import (
	"testing"

	"github.com/jjmrocha/ai-toolkit/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	_ ToolPack = (*codingTools)(nil)
	_ ToolPack = (*webTools)(nil)
)

func Test_serenaMCPConfig(t *testing.T) {
	t.Run("starts Serena on the given project with the query-projects and no-memories modes", func(t *testing.T) {
		// given
		expected := []string{
			"--from", "serena-agent==2.1.0",
			"serena", "start-mcp-server",
			"--context", "desktop-app",
			"--add-mode", "query-projects",
			"--add-mode", "no-memories",
			"--project", "ai-toolkit",
		}
		// when
		result := serenaMCPConfig("ai-toolkit", false)
		// then
		assert.Equal(t, expected, result.Args)
	})

	t.Run("leaves Serena's shell, memory, onboarding, project switching and manual tools out", func(t *testing.T) {
		// given
		expected := []string{
			"execute_shell_command",
			"write_memory", "read_memory", "list_memories",
			"edit_memory", "delete_memory", "rename_memory",
			"onboarding",
			"activate_project", "get_current_config",
			"initial_instructions",
		}
		// when
		result := serenaMCPConfig("ai-toolkit", false)
		// then
		assert.Equal(t, expected, result.ExcludedTools)
	})

	t.Run("shares nothing between calls", func(t *testing.T) {
		// given
		expected := serenaMCPConfig("ai-toolkit", false)
		// when
		result := serenaMCPConfig("ai-toolkit", false)
		result.ExcludedTools[0] = "find_symbol"
		// then
		assert.NotEqual(t, result.ExcludedTools, expected.ExcludedTools)
	})

	t.Run("registers only Serena's read tools when read-only", func(t *testing.T) {
		// given
		expected := []string{
			"read_file", "list_dir", "find_file", "search_for_pattern",
			"get_symbols_overview", "find_symbol", "find_referencing_symbols",
			"find_implementations", "find_declaration", "get_diagnostics_for_file",
			"list_queryable_projects", "query_project",
		}
		// when
		result := serenaMCPConfig("ai-toolkit", true)
		// then
		assert.Equal(t, expected, result.IncludedTools)
	})

	t.Run("registers every tool when not read-only", func(t *testing.T) {
		// when
		result := serenaMCPConfig("ai-toolkit", false)
		// then
		assert.Empty(t, result.IncludedTools)
	})

	t.Run("keeps the arguments and exclusions when read-only", func(t *testing.T) {
		// given
		expected := serenaMCPConfig("ai-toolkit", false)
		// when
		result := serenaMCPConfig("ai-toolkit", true)
		// then
		assert.Equal(t, expected.Args, result.Args)
		assert.Equal(t, expected.ExcludedTools, result.ExcludedTools)
	})

	t.Run("shares no read tool list between calls", func(t *testing.T) {
		// given
		expected := serenaMCPConfig("ai-toolkit", true)
		// when
		result := serenaMCPConfig("ai-toolkit", true)
		result.IncludedTools[0] = "create_text_file"
		// then
		assert.NotEqual(t, result.IncludedTools, expected.IncludedTools)
	})
}

func TestCodingTools(t *testing.T) {
	t.Run("returns ErrNoProject and registers nothing when the project is empty", func(t *testing.T) {
		// given
		toolBox := tools.NewToolBox()
		// when
		result, err := CodingTools(t.Context(), toolBox, "", false)
		// then
		require.ErrorIs(t, err, ErrNoProject)
		assert.Nil(t, result)
		assert.Empty(t, toolBox.Tools())
	})
}

func TestCodingToolsInstructions(t *testing.T) {
	t.Run("returns the serena guidance naming the project", func(t *testing.T) {
		// given
		pack := &codingTools{project: "ai-toolkit"}
		// when
		result := pack.Instructions(t.Context())
		// then
		assert.Equal(t, "serena", result.Name)
		assert.Contains(t, result.Text, "ai-toolkit")
	})

	t.Run("says that Serena counts lines from 0", func(t *testing.T) {
		// given
		pack := &codingTools{project: "ai-toolkit"}
		// when
		result := pack.Instructions(t.Context())
		// then
		assert.Contains(t, result.Text, "Serena counts lines from 0")
	})

	t.Run("says that serena__read_file returns no line numbers", func(t *testing.T) {
		// given
		pack := &codingTools{project: "ai-toolkit", readOnly: true}
		// when
		result := pack.Instructions(t.Context())
		// then
		assert.Contains(t, result.Text, "serena__read_file returns the text without line numbers")
	})

	t.Run("says how to fall back when serena__query_project has no project server", func(t *testing.T) {
		// given
		pack := &codingTools{project: "ai-toolkit", readOnly: true}
		// when
		result := pack.Instructions(t.Context())
		// then
		assert.Contains(t, result.Text, "Serena's project server")
		assert.Contains(t, result.Text, "back to serena__search_for_pattern")
	})

	t.Run("covers editing when not read-only", func(t *testing.T) {
		// given
		pack := &codingTools{project: "ai-toolkit"}
		// when
		result := pack.Instructions(t.Context())
		// then
		assert.Contains(t, result.Text, "serena__replace_symbol_body")
	})

	t.Run("leaves out editing when read-only", func(t *testing.T) {
		// given
		pack := &codingTools{project: "ai-toolkit", readOnly: true}
		// when
		result := pack.Instructions(t.Context())
		// then
		assert.NotContains(t, result.Text, "serena__replace_symbol_body")
		assert.Contains(t, result.Text, "Serena counts lines from 0")
	})
}

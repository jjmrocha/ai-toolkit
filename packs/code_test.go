package packs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	_ ToolPack = (*codingTools)(nil)
	_ ToolPack = (*webTools)(nil)
)

func TestSerenaMCPConfig(t *testing.T) {
	t.Run("starts Serena with the query-projects and no-memories modes", func(t *testing.T) {
		// given
		expected := []string{
			"--from", "git+https://github.com/oraios/serena",
			"serena", "start-mcp-server",
			"--context", "desktop-app",
			"--add-mode", "query-projects",
			"--add-mode", "no-memories",
		}
		// when
		result := SerenaMCPConfig("", false)
		// then
		assert.Equal(t, expected, result.Args)
	})

	t.Run("activates the given project at startup", func(t *testing.T) {
		// given
		expected := []string{
			"--from", "git+https://github.com/oraios/serena",
			"serena", "start-mcp-server",
			"--context", "desktop-app",
			"--add-mode", "query-projects",
			"--add-mode", "no-memories",
			"--project", "ai-toolkit",
		}
		// when
		result := SerenaMCPConfig("ai-toolkit", false)
		// then
		assert.Equal(t, expected, result.Args)
	})

	t.Run("leaves Serena's shell, memory and onboarding tools out", func(t *testing.T) {
		// given
		expected := []string{
			"execute_shell_command",
			"write_memory", "read_memory", "list_memories",
			"edit_memory", "delete_memory", "rename_memory",
			"onboarding",
		}
		// when
		result := SerenaMCPConfig("", false)
		// then
		assert.Equal(t, expected, result.ExcludedTools)
	})

	t.Run("shares nothing between calls", func(t *testing.T) {
		// given
		expected := SerenaMCPConfig("ai-toolkit", false)
		// when
		result := SerenaMCPConfig("ai-toolkit", false)
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
			"get_current_config", "initial_instructions", "activate_project",
		}
		// when
		result := SerenaMCPConfig("", true)
		// then
		assert.Equal(t, expected, result.IncludedTools)
	})

	t.Run("registers every tool when not read-only", func(t *testing.T) {
		// when
		result := SerenaMCPConfig("", false)
		// then
		assert.Empty(t, result.IncludedTools)
	})

	t.Run("keeps the arguments and exclusions when read-only", func(t *testing.T) {
		// given
		expected := SerenaMCPConfig("ai-toolkit", false)
		// when
		result := SerenaMCPConfig("ai-toolkit", true)
		// then
		assert.Equal(t, expected.Args, result.Args)
		assert.Equal(t, expected.ExcludedTools, result.ExcludedTools)
	})

	t.Run("shares no read tool list between calls", func(t *testing.T) {
		// given
		expected := SerenaMCPConfig("", true)
		// when
		result := SerenaMCPConfig("", true)
		result.IncludedTools[0] = "create_text_file"
		// then
		assert.NotEqual(t, result.IncludedTools, expected.IncludedTools)
	})
}

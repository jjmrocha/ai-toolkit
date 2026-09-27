package packs

import (
	"testing"

	"github.com/jjmrocha/ai-toolkit/mcp"
	"github.com/stretchr/testify/assert"
)

var _ ToolPack = (*mcp.Client)(nil)

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
		result := SerenaMCPConfig("")
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
		result := SerenaMCPConfig("ai-toolkit")
		// then
		assert.Equal(t, expected, result.Args)
	})

	t.Run("accepts a project path as well as a name", func(t *testing.T) {
		// given
		expected := []string{
			"--from", "git+https://github.com/oraios/serena",
			"serena", "start-mcp-server",
			"--context", "desktop-app",
			"--add-mode", "query-projects",
			"--add-mode", "no-memories",
			"--project", "/Users/jrocha/SOURCES/GO/ai-toolkit",
		}
		// when
		result := SerenaMCPConfig("/Users/jrocha/SOURCES/GO/ai-toolkit")
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
		result := SerenaMCPConfig("")
		// then
		assert.Equal(t, expected, result.ExcludedTools)
	})

	t.Run("shares nothing between calls", func(t *testing.T) {
		// given
		expected := SerenaMCPConfig("ai-toolkit")
		// when
		result := SerenaMCPConfig("ai-toolkit")
		result.ExcludedTools[0] = "find_symbol"
		// then
		assert.NotEqual(t, result.ExcludedTools, expected.ExcludedTools)
	})
}

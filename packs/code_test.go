package packs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSerenaMCPConfig(t *testing.T) {
	t.Run("starts Serena with the query-projects mode", func(t *testing.T) {
		// given
		expected := []string{
			"--from", "git+https://github.com/oraios/serena",
			"serena", "start-mcp-server",
			"--context", "desktop-app",
			"--add-mode", "query-projects",
		}
		// when
		result := SerenaMCPConfig()
		// then
		assert.Equal(t, expected, result.Args)
	})

	t.Run("leaves Serena's shell tool out", func(t *testing.T) {
		// given
		expected := []string{"execute_shell_command"}
		// when
		result := SerenaMCPConfig()
		// then
		assert.Equal(t, expected, result.ExcludedTools)
	})

	t.Run("shares nothing between calls", func(t *testing.T) {
		// given
		expected := SerenaMCPConfig()
		// when
		result := SerenaMCPConfig()
		result.ExcludedTools[0] = "find_symbol"
		// then
		assert.NotEqual(t, result.ExcludedTools, expected.ExcludedTools)
	})
}

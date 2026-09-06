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
}

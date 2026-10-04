package packs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var _ ToolPack = (*thinkingTools)(nil)

func TestSequentialThinkingMCPConfig(t *testing.T) {
	t.Run("starts the sequential-thinking server with npx", func(t *testing.T) {
		// given
		expected := []string{"-y", "@modelcontextprotocol/server-sequential-thinking"}
		// when
		result := SequentialThinkingMCPConfig()
		// then
		assert.Equal(t, "thinking", result.Name)
		assert.Equal(t, "npx", result.Command)
		assert.Equal(t, expected, result.Args)
	})

	t.Run("shares nothing between calls", func(t *testing.T) {
		// given
		expected := SequentialThinkingMCPConfig()
		// when
		result := SequentialThinkingMCPConfig()
		result.Args[1] = "@modelcontextprotocol/server-sequential-thinking@1.0.0"
		// then
		assert.NotEqual(t, result.Args, expected.Args)
	})
}

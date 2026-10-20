package packs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var _ ToolPack = (*libraryTools)(nil)

func TestContext7MCPConfig(t *testing.T) {
	t.Run("starts the context7 server with npx", func(t *testing.T) {
		// given
		expected := []string{"-y", "@upstash/context7-mcp@4.4.4"}
		// when
		result := Context7MCPConfig()
		// then
		assert.Equal(t, "context7", result.Name)
		assert.Equal(t, "npx", result.Command)
		assert.Equal(t, expected, result.Args)
	})

	t.Run("passes the API key variable to the server", func(t *testing.T) {
		// given
		expected := []string{"CONTEXT7_API_KEY"}
		// when
		result := Context7MCPConfig()
		// then
		assert.Equal(t, expected, result.InheritEnv)
	})

	t.Run("shares nothing between calls", func(t *testing.T) {
		// given
		expected := Context7MCPConfig()
		// when
		result := Context7MCPConfig()
		result.Args[1] = "@upstash/context7-mcp@1.0.0"
		// then
		assert.NotEqual(t, result.Args, expected.Args)
	})
}

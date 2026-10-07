package packs

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var _ ToolPack = (*webTools)(nil)

func TestDonSeTchMCPConfig(t *testing.T) {
	t.Run("starts DonSeTch's supervised MCP server", func(t *testing.T) {
		// given
		expected := []string{"mcp", "--supervised"}
		// when
		result := DonSeTchMCPConfig()
		// then
		assert.Equal(t, "donsetch", result.Name)
		assert.Equal(t, "donsetch", result.Command)
		assert.Equal(t, expected, result.Args)
	})

	t.Run("gives tool calls fifteen minutes", func(t *testing.T) {
		// when
		result := DonSeTchMCPConfig()
		// then
		assert.Equal(t, 15*time.Minute, result.ToolCallTimeout)
	})

	t.Run("shares nothing between calls", func(t *testing.T) {
		// given
		expected := DonSeTchMCPConfig()
		// when
		result := DonSeTchMCPConfig()
		result.Args[0] = "serve"
		// then
		assert.NotEqual(t, result.Args, expected.Args)
	})
}

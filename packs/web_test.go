package packs

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var _ ToolPack = (*webTools)(nil)

func TestDonSeTchMCPConfig(t *testing.T) {
	t.Run("names the server donsetch and runs it with npx", func(t *testing.T) {
		// when
		result := DonSeTchMCPConfig()
		// then
		assert.Equal(t, "donsetch", result.Name)
		assert.Equal(t, "npx", result.Command)
	})

	t.Run("excludes the screenshot tool", func(t *testing.T) {
		// when
		result := DonSeTchMCPConfig()
		// then
		assert.Contains(t, result.ExcludedTools, "web_screenshot")
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

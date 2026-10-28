package packs

import (
	"testing"

	"github.com/jjmrocha/ai-toolkit/mcp"
	"github.com/stretchr/testify/assert"
)

var _ ToolPack = (*financeTools)(nil)

func TestYFinanceMCPConfig(t *testing.T) {
	t.Run("starts the yfinance server with uvx", func(t *testing.T) {
		// given
		expected := []string{"--from", "yfmcp==0.14.0", "yfmcp"}
		// when
		result := YFinanceMCPConfig()
		// then
		assert.Equal(t, "yfinance", result.Name)
		assert.Equal(t, "uvx", result.Command)
		assert.Equal(t, expected, result.Args)
	})

	t.Run("shares nothing between calls", func(t *testing.T) {
		// given
		expected := YFinanceMCPConfig()
		// when
		result := YFinanceMCPConfig()
		result.Args[1] = "yfmcp==0.1.0"
		// then
		assert.NotEqual(t, result.Args, expected.Args)
	})
}

func TestFinanceToolsInstructions(t *testing.T) {
	t.Run("returns the yfinance guidance", func(t *testing.T) {
		// given
		pack := &financeTools{}
		// when
		result := pack.Instructions(t.Context())
		// then
		expected := &mcp.Instruction{Name: "yfinance", Text: yfinanceInstruction}
		assert.Equal(t, expected, result)
	})
}

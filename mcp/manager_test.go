package mcp

import (
	"context"
	"testing"

	"github.com/jjmrocha/ai-toolkit/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	t.Run("starts with nothing registered", func(t *testing.T) {
		// given
		tb := tools.NewToolBox()
		// when
		result := NewManager(tb)
		// then
		require.NotNil(t, result)
		assert.Empty(t, result.Status())
	})
}

func TestManagerRegister(t *testing.T) {
	t.Run("makes the MCP known without starting it", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		cfg := ClientConfig{Name: "playwright", Command: "npx"}
		// when
		m.Register(cfg)
		// then
		expected := []Status{{Name: "playwright", Active: false}}
		assert.Equal(t, expected, m.Status())
	})

	t.Run("replaces the configuration of a name already registered", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		m.Register(ClientConfig{Name: "playwright", Command: "npx"})
		// when
		m.Register(ClientConfig{Name: "playwright", Command: "bunx"})
		// then
		expected := []Status{{Name: "playwright", Active: false}}
		assert.Equal(t, expected, m.Status())
	})
}

func TestManagerStatus(t *testing.T) {
	t.Run("reports every registered MCP", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		m.Register(ClientConfig{Name: "playwright", Command: "npx"})
		m.Register(ClientConfig{Name: "serena", Command: "uvx"})
		// when
		result := m.Status()
		// then
		assert.Len(t, result, 2)
		assert.Contains(t, result, Status{Name: "playwright", Active: false})
		assert.Contains(t, result, Status{Name: "serena", Active: false})
	})
}

func TestManagerStart(t *testing.T) {
	t.Run("refuses a name that was never registered", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		ctx := context.Background()
		// when
		err := m.Start(ctx, "playwright")
		// then
		assert.ErrorIs(t, err, ErrMCPNotRegistered)
	})
}

func TestManagerStop(t *testing.T) {
	t.Run("refuses a name that was never registered", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		// when
		err := m.Stop("playwright")
		// then
		assert.ErrorIs(t, err, ErrMCPNotRegistered)
	})

	t.Run("accepts a registered MCP that is not running", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		m.Register(ClientConfig{Name: "playwright", Command: "npx"})
		// when
		err := m.Stop("playwright")
		// then
		require.NoError(t, err)
		expected := []Status{{Name: "playwright", Active: false}}
		assert.Equal(t, expected, m.Status())
	})
}

func TestManagerClose(t *testing.T) {
	t.Run("keeps the registrations so they can be started again", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		m.Register(ClientConfig{Name: "playwright", Command: "npx"})
		// when
		m.Close()
		// then
		expected := []Status{{Name: "playwright", Active: false}}
		assert.Equal(t, expected, m.Status())
	})
}

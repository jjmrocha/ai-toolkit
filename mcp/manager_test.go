package mcp

import (
	"testing"
	"time"

	"github.com/jjmrocha/ai-toolkit/tools"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
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

func TestManagerInstructions(t *testing.T) {
	t.Run("returns the instructions of the running MCPs, sorted by name", func(t *testing.T) {
		// given
		startTestMCPServer(t, []string{"search"}, &sdk.ServerOptions{Instructions: "Use search to query the web."}, "zeta")
		startTestMCPServer(t, []string{"fetch"}, &sdk.ServerOptions{Instructions: "Use fetch to read a page."}, "alpha")
		ctx := t.Context()
		m := NewManager(tools.NewToolBox())
		m.Register(ClientConfig{Name: "zeta", Command: "npx"})
		m.Register(ClientConfig{Name: "alpha", Command: "npx"})
		t.Cleanup(m.Close)
		require.NoError(t, m.Start(ctx, "zeta"))
		require.NoError(t, m.Start(ctx, "alpha"))
		// when
		result := m.Instructions()
		// then
		expected := []Instruction{
			{Name: "alpha", Text: "Use fetch to read a page."},
			{Name: "zeta", Text: "Use search to query the web."},
		}
		assert.Equal(t, expected, result)
	})

	t.Run("leaves out an MCP that sent no instructions", func(t *testing.T) {
		// given
		startTestMCPServer(t, []string{"search"}, &sdk.ServerOptions{Instructions: "Use search to query the web."}, "playwright")
		startTestMCPServer(t, []string{"fetch"}, nil, "quiet")
		ctx := t.Context()
		m := NewManager(tools.NewToolBox())
		m.Register(ClientConfig{Name: "playwright", Command: "npx"})
		m.Register(ClientConfig{Name: "quiet", Command: "npx"})
		t.Cleanup(m.Close)
		require.NoError(t, m.Start(ctx, "playwright"))
		require.NoError(t, m.Start(ctx, "quiet"))
		// when
		result := m.Instructions()
		// then
		expected := []Instruction{{Name: "playwright", Text: "Use search to query the web."}}
		assert.Equal(t, expected, result)
	})

	t.Run("leaves out an MCP that is not running", func(t *testing.T) {
		// given
		startTestMCPServer(t, []string{"search"}, &sdk.ServerOptions{Instructions: "Use search to query the web."})
		ctx := t.Context()
		m := NewManager(tools.NewToolBox())
		m.Register(ClientConfig{Name: "playwright", Command: "npx"})
		m.Register(ClientConfig{Name: "registered-only", Command: "npx"})
		t.Cleanup(m.Close)
		require.NoError(t, m.Start(ctx, "playwright"))
		// when
		result := m.Instructions()
		// then
		expected := []Instruction{{Name: "playwright", Text: "Use search to query the web."}}
		assert.Equal(t, expected, result)
	})

	t.Run("reaps a dead client and leaves it out", func(t *testing.T) {
		// given
		server := startTestMCPServer(t, []string{"search"}, &sdk.ServerOptions{Instructions: "Use search to query the web."}, "playwright")
		ctx := t.Context()
		toolBox := tools.NewToolBox()
		m := NewManager(toolBox)
		m.Register(ClientConfig{Name: "playwright", Command: "npx"})
		t.Cleanup(m.Close)
		require.NoError(t, m.Start(ctx, "playwright"))
		require.NoError(t, server.session.Close())
		assert.Eventually(t, func() bool {
			return len(toolBox.Tools()) == 0
		}, time.Second, 10*time.Millisecond)
		// when
		result := m.Instructions()
		// then
		assert.Nil(t, result)
		expected := []Status{{Name: "playwright", Active: false}}
		assert.Equal(t, expected, m.Status())
	})

	t.Run("nil when no running MCP sent instructions", func(t *testing.T) {
		// given
		startTestMCPServer(t, []string{"search"}, nil)
		ctx := t.Context()
		m := NewManager(tools.NewToolBox())
		m.Register(ClientConfig{Name: "playwright", Command: "npx"})
		t.Cleanup(m.Close)
		require.NoError(t, m.Start(ctx, "playwright"))
		// when
		result := m.Instructions()
		// then
		assert.Nil(t, result)
	})

	t.Run("nil when nothing is running", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		m.Register(ClientConfig{Name: "playwright", Command: "npx"})
		// when
		result := m.Instructions()
		// then
		assert.Nil(t, result)
	})
}

func TestManagerStart(t *testing.T) {
	t.Run("refuses a name that was never registered", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		ctx := t.Context()
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

	t.Run("keeps the registration of a server it started", func(t *testing.T) {
		// given
		startTestMCPServer(t, []string{"search"}, nil)
		ctx := t.Context()
		m := NewManager(tools.NewToolBox())
		m.Register(ClientConfig{Name: "playwright", Command: "npx"})
		require.NoError(t, m.Start(ctx, "playwright"))
		m.Close()
		t.Cleanup(m.Close)
		// when
		err := m.Start(ctx, "playwright")
		// then
		require.NoError(t, err)
		expected := []Status{{Name: "playwright", Active: true}}
		assert.Equal(t, expected, m.Status())
	})
}

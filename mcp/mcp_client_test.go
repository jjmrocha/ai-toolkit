package mcp

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jjmrocha/ai-toolkit/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	testCases := []struct {
		name     string
		config   ClientConfig
		expected error
	}{
		{
			name:     "missing name",
			config:   ClientConfig{Command: "npx"},
			expected: ErrNameRequired,
		},
		{
			name:     "missing command",
			config:   ClientConfig{Name: "playwright"},
			expected: ErrCommandRequired,
		},
		{
			name:     "missing both",
			config:   ClientConfig{},
			expected: ErrNameRequired,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// given
			ctx := context.Background()
			// when
			result, err := NewClient(ctx, tc.config)
			// then
			assert.Nil(t, result)
			assert.ErrorIs(t, err, tc.expected)
		})
	}
}

func TestClientToolName(t *testing.T) {
	t.Run("namespaces the tool under the server name", func(t *testing.T) {
		// given
		c := &Client{config: ClientConfig{Name: "playwright"}}
		// when
		result := c.toolName("browser_navigate", nil)
		// then
		expected := "playwright__browser_navigate"
		assert.Equal(t, expected, result)
	})

	t.Run("replaces characters the providers reject", func(t *testing.T) {
		// given
		c := &Client{config: ClientConfig{Name: "web.tools"}}
		// when
		result := c.toolName("fetch/page", nil)
		// then
		expected := "web_tools__fetch_page"
		assert.Equal(t, expected, result)
		assert.True(t, tools.ValidToolName(result))
	})

	t.Run("shortens a name the providers would reject for length", func(t *testing.T) {
		// given
		c := &Client{config: ClientConfig{Name: strings.Repeat("a", 40)}}
		// when
		result := c.toolName(strings.Repeat("b", 40), nil)
		// then
		assert.Len(t, result, tools.MaxToolNameLength)
		assert.True(t, tools.ValidToolName(result))
	})

	t.Run("gives a name already taken a distinct suffix", func(t *testing.T) {
		// given
		c := &Client{config: ClientConfig{Name: "playwright"}}
		taken := []string{"playwright__browser_navigate"}
		// when
		result := c.toolName("browser_navigate", taken)
		// then
		assert.NotContains(t, taken, result)
		assert.True(t, tools.ValidToolName(result))
	})

	t.Run("gives the same tool the same name every time", func(t *testing.T) {
		// given
		c := &Client{config: ClientConfig{Name: strings.Repeat("a", 40)}}
		expected := c.toolName(strings.Repeat("b", 40), nil)
		// when
		result := c.toolName(strings.Repeat("b", 40), nil)
		// then
		assert.Equal(t, expected, result)
	})

	t.Run("gives different tools different names once shortened", func(t *testing.T) {
		// given
		c := &Client{config: ClientConfig{Name: strings.Repeat("a", 40)}}
		first := c.toolName(strings.Repeat("b", 40), nil)
		// when
		result := c.toolName(strings.Repeat("c", 40), nil)
		// then
		assert.NotEqual(t, first, result)
	})
}

func TestClientExcluded(t *testing.T) {
	t.Run("keeps a tool no config names", func(t *testing.T) {
		// given
		c := &Client{config: ClientConfig{Name: "serena"}}
		// when
		result := c.excluded("find_symbol")
		// then
		assert.False(t, result)
	})

	t.Run("drops a tool the config names", func(t *testing.T) {
		// given
		c := &Client{config: ClientConfig{
			Name:          "serena",
			ExcludedTools: []string{"execute_shell_command"},
		}}
		// when
		result := c.excluded("execute_shell_command")
		// then
		assert.True(t, result)
	})

	t.Run("keeps a tool the config does not name", func(t *testing.T) {
		// given
		c := &Client{config: ClientConfig{
			Name:          "serena",
			ExcludedTools: []string{"execute_shell_command"},
		}}
		// when
		result := c.excluded("find_symbol")
		// then
		assert.False(t, result)
	})

	t.Run("matches the name the server published, not the namespaced one", func(t *testing.T) {
		// given
		c := &Client{config: ClientConfig{
			Name:          "serena",
			ExcludedTools: []string{"serena__execute_shell_command"},
		}}
		// when
		result := c.excluded("execute_shell_command")
		// then
		assert.False(t, result)
	})
}

func TestHashToolName(t *testing.T) {
	t.Run("is stable for the same input", func(t *testing.T) {
		// given
		expected := hashToolName("playwright__browser_navigate")
		// when
		result := hashToolName("playwright__browser_navigate")
		// then
		require.Len(t, result, toolNameHashLength)
		assert.Equal(t, expected, result)
	})

	t.Run("differs for different input", func(t *testing.T) {
		// given
		first := hashToolName("playwright__browser_navigate")
		// when
		result := hashToolName("playwright__browser_click")
		// then
		assert.NotEqual(t, first, result)
	})
}

func TestClientConnected(t *testing.T) {
	t.Run("reports false and removes the tools once the server exits", func(t *testing.T) {
		// given
		server := startTestMCPServer(t, "search")
		ctx := context.Background()
		toolBox := tools.NewToolBox()
		client, err := NewClient(ctx, ClientConfig{Name: "playwright", Command: "npx"})
		require.NoError(t, err)
		t.Cleanup(func() { _ = client.Close() })
		require.NoError(t, client.RegisterTools(ctx, toolBox))
		require.Equal(t, []string{"playwright__search"}, registeredToolNames(toolBox))
		// when
		require.NoError(t, server.session.Close())
		// then
		assert.Eventually(t, func() bool {
			return !client.Connected() && len(toolBox.Tools()) == 0
		}, time.Second, 10*time.Millisecond)
	})
}

func TestClientRegisterTools(t *testing.T) {
	t.Run("follows the server's tool list when it changes", func(t *testing.T) {
		// given
		server := startTestMCPServer(t, "search", "fetch")
		ctx := context.Background()
		toolBox := tools.NewToolBox()
		client, err := NewClient(ctx, ClientConfig{Name: "playwright", Command: "npx"})
		require.NoError(t, err)
		t.Cleanup(func() { _ = client.Close() })
		require.NoError(t, client.RegisterTools(ctx, toolBox))
		expected := []string{"playwright__fetch", "playwright__search"}
		require.Equal(t, expected, registeredToolNames(toolBox))
		// when
		server.server.RemoveTools("fetch")
		// then
		assert.Eventually(t, func() bool {
			return slices.Equal([]string{"playwright__search"}, registeredToolNames(toolBox))
		}, time.Second, 10*time.Millisecond)
	})
}

package mcp

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/tools"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// given
			ctx := t.Context()
			// when
			result, err := NewClient(ctx, tc.config, tools.NewToolBox())
			// then
			assert.Nil(t, result)
			assert.ErrorIs(t, err, tc.expected)
		})
	}

	t.Run("registers the server's tools under its name", func(t *testing.T) {
		// given
		startTestMCPServer(t, []string{"search", "fetch"}, nil)
		ctx := t.Context()
		toolBox := tools.NewToolBox()
		// when
		client, err := NewClient(ctx, ClientConfig{Name: "playwright", Command: "npx"}, toolBox)
		// then
		require.NoError(t, err)
		t.Cleanup(func() { _ = client.Close() })
		expected := []string{"playwright__fetch", "playwright__search"}
		assert.Equal(t, expected, registeredToolNames(toolBox))
	})

	t.Run("registers nothing with a nil ToolBox and still calls tools", func(t *testing.T) {
		// given
		startTestMCPServer(t, []string{"search"}, nil)
		ctx := t.Context()
		client, err := NewClient(ctx, ClientConfig{Name: "playwright", Command: "npx"}, nil)
		require.NoError(t, err)
		t.Cleanup(func() { _ = client.Close() })
		// when
		result, err := client.CallTool(ctx, "search", nil)
		// then
		require.NoError(t, err)
		assert.Equal(t, "ok", result)
	})

	t.Run("stops the server when its tools cannot be listed", func(t *testing.T) {
		// given
		server := startTestMCPServer(t, []string{"search"}, nil)
		server.server.AddReceivingMiddleware(failToolsList)
		ctx := t.Context()
		toolBox := tools.NewToolBox()
		// when
		result, err := NewClient(ctx, ClientConfig{Name: "playwright", Command: "npx"}, toolBox)
		// then
		assert.Nil(t, result)
		assert.Error(t, err)
		assert.Empty(t, toolBox.Tools())
		closed := make(chan struct{})
		go func() {
			_ = server.session.Wait()
			close(closed)
		}()
		assert.Eventually(t, func() bool {
			select {
			case <-closed:
				return true
			default:
				return false
			}
		}, time.Second, 10*time.Millisecond)
	})

	t.Run("follows the server's tool list when it changes", func(t *testing.T) {
		// given
		server := startTestMCPServer(t, []string{"search", "fetch"}, nil)
		ctx := t.Context()
		toolBox := tools.NewToolBox()
		client, err := NewClient(ctx, ClientConfig{Name: "playwright", Command: "npx"}, toolBox)
		require.NoError(t, err)
		t.Cleanup(func() { _ = client.Close() })
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

func TestClientInstructions(t *testing.T) {
	t.Run("returns the instructions the server sent at handshake", func(t *testing.T) {
		// given
		startTestMCPServer(t, []string{"search"}, &sdk.ServerOptions{Instructions: "Use search to query the web."})
		ctx := t.Context()
		client, err := NewClient(ctx, ClientConfig{Name: "playwright", Command: "npx"}, tools.NewToolBox())
		require.NoError(t, err)
		t.Cleanup(func() { _ = client.Close() })
		// when
		result := client.Instructions()
		// then
		expected := &Instruction{Name: "playwright", Text: "Use search to query the web."}
		assert.Equal(t, expected, result)
	})

	t.Run("nil when the server sent none", func(t *testing.T) {
		// given
		startTestMCPServer(t, []string{"search"}, nil)
		ctx := t.Context()
		client, err := NewClient(ctx, ClientConfig{Name: "playwright", Command: "npx"}, tools.NewToolBox())
		require.NoError(t, err)
		t.Cleanup(func() { _ = client.Close() })
		// when
		result := client.Instructions()
		// then
		assert.Nil(t, result)
	})
}

func TestClientName(t *testing.T) {
	t.Run("returns the name the client registered under", func(t *testing.T) {
		// given
		startTestMCPServer(t, []string{"search"}, nil)
		ctx := t.Context()
		client, err := NewClient(ctx, ClientConfig{Name: "playwright", Command: "npx"}, tools.NewToolBox())
		require.NoError(t, err)
		t.Cleanup(func() { _ = client.Close() })
		// when
		result := client.Name()
		// then
		assert.Equal(t, "playwright", result)
	})
}

func TestClientCallTool(t *testing.T) {
	t.Run("returns the text the server's tool produced", func(t *testing.T) {
		// given
		startTestMCPServer(t, []string{"search"}, nil)
		ctx := t.Context()
		client, err := NewClient(ctx, ClientConfig{Name: "playwright", Command: "npx"}, tools.NewToolBox())
		require.NoError(t, err)
		t.Cleanup(func() { _ = client.Close() })
		// when
		result, err := client.CallTool(ctx, "search", nil)
		// then
		require.NoError(t, err)
		assert.Equal(t, "ok", result)
	})

	t.Run("fails for a tool the server does not have", func(t *testing.T) {
		// given
		startTestMCPServer(t, []string{"search"}, nil)
		ctx := t.Context()
		client, err := NewClient(ctx, ClientConfig{Name: "playwright", Command: "npx"}, tools.NewToolBox())
		require.NoError(t, err)
		t.Cleanup(func() { _ = client.Close() })
		// when
		_, err = client.CallTool(ctx, "fetch", nil)
		// then
		assert.Error(t, err)
	})
}

func TestClientConnected(t *testing.T) {
	t.Run("reports false and removes the tools once the server exits", func(t *testing.T) {
		// given
		server := startTestMCPServer(t, []string{"search"}, nil)
		ctx := t.Context()
		toolBox := tools.NewToolBox()
		client, err := NewClient(ctx, ClientConfig{Name: "playwright", Command: "npx"}, toolBox)
		require.NoError(t, err)
		t.Cleanup(func() { _ = client.Close() })
		require.Equal(t, []string{"playwright__search"}, registeredToolNames(toolBox))
		// when
		require.NoError(t, server.session.Close())
		// then
		assert.Eventually(t, func() bool {
			return !client.Connected() && len(toolBox.Tools()) == 0
		}, time.Second, 10*time.Millisecond)
	})
}

func TestClientClose(t *testing.T) {
	t.Run("removes only the client's tools from the ToolBox", func(t *testing.T) {
		// given
		startTestMCPServer(t, []string{"search"}, nil)
		toolBox := tools.NewToolBox()
		require.NoError(t, toolBox.Add(llm.Tool{Name: "other"}, testHandler))
		client, err := NewClient(t.Context(), ClientConfig{Name: "playwright", Command: "npx"}, toolBox)
		require.NoError(t, err)
		// when
		err = client.Close()
		// then
		assert.NoError(t, err)
		expected := []string{"other"}
		assert.Equal(t, expected, registeredToolNames(toolBox))
	})

	t.Run("leaves a newer client's tools in place on a second close", func(t *testing.T) {
		// given
		startTestMCPServer(t, []string{"search"}, nil)
		toolBox := tools.NewToolBox()
		cfg := ClientConfig{Name: "playwright", Command: "npx"}
		oldClient, err := NewClient(t.Context(), cfg, toolBox)
		require.NoError(t, err)
		require.NoError(t, oldClient.Close())
		newClient, err := NewClient(t.Context(), cfg, toolBox)
		require.NoError(t, err)
		t.Cleanup(func() { _ = newClient.Close() })
		// when
		err = oldClient.Close()
		// then
		assert.NoError(t, err)
		expected := []string{"playwright__search"}
		assert.Equal(t, expected, registeredToolNames(toolBox))
	})
}

func testHandler(context.Context, map[string]any) (string, error) {
	return "", nil
}

func failToolsList(next sdk.MethodHandler) sdk.MethodHandler {
	return func(ctx context.Context, method string, req sdk.Request) (sdk.Result, error) {
		if method == "tools/list" {
			return nil, errors.New("list failed")
		}

		return next(ctx, method, req)
	}
}

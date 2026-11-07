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
		server := startTestMCPServer(t, []string{"search", "fetch"}, nil)
		ctx := t.Context()
		toolBox := tools.NewToolBox()
		// when
		client, err := newClient(ctx, ClientConfig{Name: "playwright", Command: "npx"}, toolBox, testTransport(t, server))
		// then
		require.NoError(t, err)
		t.Cleanup(client.Close)
		expected := []string{"playwright__fetch", "playwright__search"}
		assert.Equal(t, expected, registeredToolNames(toolBox))
	})

	t.Run("registers the server's tools under their own names", func(t *testing.T) {
		// given
		server := startTestMCPServer(t, []string{"search", "fetch"}, nil)
		ctx := t.Context()
		toolBox := tools.NewToolBox()
		cfg := ClientConfig{Name: "playwright", Command: "npx", NoToolPrefix: true}
		// when
		client, err := newClient(ctx, cfg, toolBox, testTransport(t, server))
		// then
		require.NoError(t, err)
		t.Cleanup(client.Close)
		expected := []string{"fetch", "search"}
		assert.Equal(t, expected, registeredToolNames(toolBox))
	})

	t.Run("fails when a tool name is already taken", func(t *testing.T) {
		// given
		server := startTestMCPServer(t, []string{"search", "fetch"}, nil)
		ctx := t.Context()
		toolBox := tools.NewToolBox()
		require.NoError(t, toolBox.Add(llm.Tool{Name: "search"}, testHandler))
		cfg := ClientConfig{Name: "playwright", Command: "npx", NoToolPrefix: true}
		// when
		result, err := newClient(ctx, cfg, toolBox, testTransport(t, server))
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrToolNameTaken)
		expected := []string{"search"}
		assert.Equal(t, expected, registeredToolNames(toolBox))
	})

	t.Run("keeps today's behavior for a name already taken when the prefix is on", func(t *testing.T) {
		// given
		server := startTestMCPServer(t, []string{"search"}, nil)
		ctx := t.Context()
		toolBox := tools.NewToolBox()
		require.NoError(t, toolBox.Add(llm.Tool{Name: "playwright__search"}, testHandler))
		// when
		client, err := newClient(ctx, ClientConfig{Name: "playwright", Command: "npx"}, toolBox, testTransport(t, server))
		// then
		require.NoError(t, err)
		t.Cleanup(client.Close)
	})

	t.Run("registers nothing with a nil ToolBox and still calls tools", func(t *testing.T) {
		// given
		server := startTestMCPServer(t, []string{"search"}, nil)
		ctx := t.Context()
		client, err := newClient(ctx, ClientConfig{Name: "playwright", Command: "npx"}, nil, testTransport(t, server))
		require.NoError(t, err)
		t.Cleanup(client.Close)
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
		result, err := newClient(ctx, ClientConfig{Name: "playwright", Command: "npx"}, toolBox, testTransport(t, server))
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
		client, err := newClient(ctx, ClientConfig{Name: "playwright", Command: "npx"}, toolBox, testTransport(t, server))
		require.NoError(t, err)
		t.Cleanup(client.Close)
		expected := []string{"playwright__fetch", "playwright__search"}
		require.Equal(t, expected, registeredToolNames(toolBox))
		// when
		server.server.RemoveTools("fetch")
		// then
		assert.Eventually(t, func() bool {
			return slices.Equal([]string{"playwright__search"}, registeredToolNames(toolBox))
		}, time.Second, 10*time.Millisecond)
	})

	t.Run("registers only the included tools, also when the server's list changes", func(t *testing.T) {
		// given
		server := startTestMCPServer(t, []string{"search", "fetch"}, nil)
		ctx := t.Context()
		toolBox := tools.NewToolBox()
		config := ClientConfig{Name: "playwright", Command: "npx", IncludedTools: []string{"search", "crawl"}}
		client, err := newClient(ctx, config, toolBox, testTransport(t, server))
		require.NoError(t, err)
		t.Cleanup(client.Close)
		require.Equal(t, []string{"playwright__search"}, registeredToolNames(toolBox))
		// when
		server.server.AddTool(testTool("extra"), testToolHandler)
		server.server.AddTool(testTool("crawl"), testToolHandler)
		// then
		expected := []string{"playwright__crawl", "playwright__search"}
		assert.Eventually(t, func() bool {
			return slices.Equal(expected, registeredToolNames(toolBox))
		}, time.Second, 10*time.Millisecond)
	})

	t.Run("replaces characters the providers reject", func(t *testing.T) {
		// given
		cfg := ClientConfig{Name: "web.tools", Command: "npx"}
		// when
		result := registerServerTools(t, cfg, "fetch.page")
		// then
		expected := []string{"web_tools__fetch_page"}
		assert.Equal(t, expected, result)
	})

	t.Run("shortens a name the providers would reject for length", func(t *testing.T) {
		// given
		cfg := ClientConfig{Name: strings.Repeat("a", 40), Command: "npx"}
		// when
		result := registerServerTools(t, cfg, strings.Repeat("b", 40))
		// then
		require.Len(t, result, 1)
		assert.Len(t, result[0], tools.MaxToolNameLength)
		assert.True(t, tools.ValidToolName(result[0]))
	})

	t.Run("still shortens an unprefixed name the providers would reject for length", func(t *testing.T) {
		// given
		cfg := ClientConfig{Name: "playwright", Command: "npx", NoToolPrefix: true}
		// when
		result := registerServerTools(t, cfg, strings.Repeat("b", 70))
		// then
		require.Len(t, result, 1)
		assert.Len(t, result[0], tools.MaxToolNameLength)
		assert.True(t, tools.ValidToolName(result[0]))
	})

	t.Run("gives two tools that sanitize to the same name distinct names", func(t *testing.T) {
		// given
		cfg := ClientConfig{Name: "playwright", Command: "npx"}
		// when
		result := registerServerTools(t, cfg, "fetch.page", "fetch_page")
		// then
		require.Len(t, result, 2)
		assert.NotEqual(t, result[0], result[1])
		assert.True(t, tools.ValidToolName(result[0]))
		assert.True(t, tools.ValidToolName(result[1]))
	})

	t.Run("gives the same tool the same name every time", func(t *testing.T) {
		// given
		cfg := ClientConfig{Name: strings.Repeat("a", 40), Command: "npx"}
		expected := registerServerTools(t, cfg, strings.Repeat("b", 40))
		// when
		result := registerServerTools(t, cfg, strings.Repeat("b", 40))
		// then
		assert.Equal(t, expected, result)
	})

	t.Run("gives different tools different names once shortened", func(t *testing.T) {
		// given
		cfg := ClientConfig{Name: strings.Repeat("a", 40), Command: "npx"}
		// when
		result := registerServerTools(t, cfg, strings.Repeat("b", 40), strings.Repeat("c", 40))
		// then
		require.Len(t, result, 2)
		assert.NotEqual(t, result[0], result[1])
	})

	t.Run("leaves out the excluded tools and keeps the rest", func(t *testing.T) {
		// given
		cfg := ClientConfig{Name: "serena", Command: "uvx", ExcludedTools: []string{"execute_shell_command"}}
		// when
		result := registerServerTools(t, cfg, "execute_shell_command", "find_symbol")
		// then
		expected := []string{"serena__find_symbol"}
		assert.Equal(t, expected, result)
	})

	t.Run("excludes by the name the server published, not the namespaced one", func(t *testing.T) {
		// given
		cfg := ClientConfig{Name: "serena", Command: "uvx", ExcludedTools: []string{"serena__execute_shell_command"}}
		// when
		result := registerServerTools(t, cfg, "execute_shell_command")
		// then
		expected := []string{"serena__execute_shell_command"}
		assert.Equal(t, expected, result)
	})

	t.Run("keeps every tool when the include list is empty", func(t *testing.T) {
		// given
		cfg := ClientConfig{Name: "serena", Command: "uvx", IncludedTools: []string{}}
		// when
		result := registerServerTools(t, cfg, "replace_content")
		// then
		expected := []string{"serena__replace_content"}
		assert.Equal(t, expected, result)
	})

	t.Run("leaves out a tool both lists name", func(t *testing.T) {
		// given
		cfg := ClientConfig{
			Name:          "serena",
			Command:       "uvx",
			IncludedTools: []string{"execute_shell_command"},
			ExcludedTools: []string{"execute_shell_command"},
		}
		// when
		result := registerServerTools(t, cfg, "execute_shell_command")
		// then
		assert.Empty(t, result)
	})

	t.Run("includes by the name the server published, not the namespaced one", func(t *testing.T) {
		// given
		cfg := ClientConfig{Name: "serena", Command: "uvx", IncludedTools: []string{"serena__find_symbol"}}
		// when
		result := registerServerTools(t, cfg, "find_symbol")
		// then
		assert.Empty(t, result)
	})
}

func registerServerTools(t *testing.T, cfg ClientConfig, toolNames ...string) []string {
	t.Helper()

	server := startTestMCPServer(t, toolNames, nil)
	toolBox := tools.NewToolBox()

	client, err := newClient(t.Context(), cfg, toolBox, testTransport(t, server))
	require.NoError(t, err)
	t.Cleanup(client.Close)

	return registeredToolNames(toolBox)
}

func TestClientInstructions(t *testing.T) {
	t.Run("returns the instructions the server sent at handshake", func(t *testing.T) {
		// given
		server := startTestMCPServer(t, []string{"search"}, &sdk.ServerOptions{Instructions: "Use search to query the web."})
		ctx := t.Context()
		client, err := newClient(ctx, ClientConfig{Name: "playwright", Command: "npx"}, tools.NewToolBox(), testTransport(t, server))
		require.NoError(t, err)
		t.Cleanup(client.Close)
		// when
		result := client.Instructions()
		// then
		expected := &Instruction{Name: "playwright", Text: "Use search to query the web."}
		assert.Equal(t, expected, result)
	})

	t.Run("nil when the server sent none", func(t *testing.T) {
		// given
		server := startTestMCPServer(t, []string{"search"}, nil)
		ctx := t.Context()
		client, err := newClient(ctx, ClientConfig{Name: "playwright", Command: "npx"}, tools.NewToolBox(), testTransport(t, server))
		require.NoError(t, err)
		t.Cleanup(client.Close)
		// when
		result := client.Instructions()
		// then
		assert.Nil(t, result)
	})
}

func TestClientName(t *testing.T) {
	t.Run("returns the name the client registered under", func(t *testing.T) {
		// given
		server := startTestMCPServer(t, []string{"search"}, nil)
		ctx := t.Context()
		client, err := newClient(ctx, ClientConfig{Name: "playwright", Command: "npx"}, tools.NewToolBox(), testTransport(t, server))
		require.NoError(t, err)
		t.Cleanup(client.Close)
		// when
		result := client.Name()
		// then
		assert.Equal(t, "playwright", result)
	})
}

func TestClientCallTool(t *testing.T) {
	t.Run("returns the text the server's tool produced", func(t *testing.T) {
		// given
		server := startTestMCPServer(t, []string{"search"}, nil)
		ctx := t.Context()
		client, err := newClient(ctx, ClientConfig{Name: "playwright", Command: "npx"}, tools.NewToolBox(), testTransport(t, server))
		require.NoError(t, err)
		t.Cleanup(client.Close)
		// when
		result, err := client.CallTool(ctx, "search", nil)
		// then
		require.NoError(t, err)
		assert.Equal(t, "ok", result)
	})

	t.Run("fails for a tool the server does not have", func(t *testing.T) {
		// given
		server := startTestMCPServer(t, []string{"search"}, nil)
		ctx := t.Context()
		client, err := newClient(ctx, ClientConfig{Name: "playwright", Command: "npx"}, tools.NewToolBox(), testTransport(t, server))
		require.NoError(t, err)
		t.Cleanup(client.Close)
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
		client, err := newClient(ctx, ClientConfig{Name: "playwright", Command: "npx"}, toolBox, testTransport(t, server))
		require.NoError(t, err)
		t.Cleanup(client.Close)
		require.Equal(t, []string{"playwright__search"}, registeredToolNames(toolBox))
		// when
		require.NoError(t, server.session.Close())
		// then
		assert.Eventually(t, func() bool {
			return !client.Connected() && len(toolBox.Tools()) == 0
		}, time.Second, 10*time.Millisecond)
	})

	t.Run("registers nothing when the tool list changes after the client disconnected", func(t *testing.T) {
		// given
		server := startTestMCPServer(t, []string{"search"}, nil)
		toolBox := tools.NewToolBox()
		client, err := newClient(t.Context(), ClientConfig{Name: "playwright", Command: "npx"}, toolBox, testTransport(t, server))
		require.NoError(t, err)
		t.Cleanup(client.Close)
		client.disconnected()
		// when
		client.onToolsChange()
		// then
		assert.Empty(t, toolBox.Tools())
	})
}

func TestClientClose(t *testing.T) {
	t.Run("removes only the client's tools from the ToolBox", func(t *testing.T) {
		// given
		server := startTestMCPServer(t, []string{"search"}, nil)
		toolBox := tools.NewToolBox()
		require.NoError(t, toolBox.Add(llm.Tool{Name: "other"}, testHandler))
		client, err := newClient(t.Context(), ClientConfig{Name: "playwright", Command: "npx"}, toolBox, testTransport(t, server))
		require.NoError(t, err)
		// when
		client.Close()
		// then
		expected := []string{"other"}
		assert.Equal(t, expected, registeredToolNames(toolBox))
	})

	t.Run("leaves a newer client's tools in place on a second close", func(t *testing.T) {
		// given
		server := startTestMCPServer(t, []string{"search"}, nil)
		toolBox := tools.NewToolBox()
		cfg := ClientConfig{Name: "playwright", Command: "npx"}
		oldClient, err := newClient(t.Context(), cfg, toolBox, testTransport(t, server))
		require.NoError(t, err)
		oldClient.Close()
		newerClient, err := newClient(t.Context(), cfg, toolBox, testTransport(t, server))
		require.NoError(t, err)
		t.Cleanup(newerClient.Close)
		// when
		oldClient.Close()
		// then
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

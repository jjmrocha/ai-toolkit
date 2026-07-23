package mcp

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMemClient(name string, responses ...string) (*Client, *tools.ToolBox, *bytes.Buffer) {
	in := &bytes.Buffer{}
	tb := tools.NewToolBox()
	c := &Client{
		config:    ClientConfig{Name: name},
		transport: &stdio{in: in, out: bufio.NewReader(strings.NewReader(strings.Join(responses, "")))},
	}
	return c, tb, in
}

func TestNewClient(t *testing.T) {
	t.Run("returns ErrNameRequired when the name is empty", func(t *testing.T) {
		// when
		result, err := NewClient(t.Context(), ClientConfig{Command: "server"})
		// then
		assert.ErrorIs(t, err, ErrNameRequired)
		assert.Nil(t, result)
	})

	t.Run("returns ErrCommandRequired when the command is empty", func(t *testing.T) {
		// when
		result, err := NewClient(t.Context(), ClientConfig{Name: "srv"})
		// then
		assert.ErrorIs(t, err, ErrCommandRequired)
		assert.Nil(t, result)
	})
}

func TestRegisterTools(t *testing.T) {
	t.Run("registers each tool namespaced with the client name", func(t *testing.T) {
		// given
		c, tb, _ := newMemClient("srv",
			`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"echo","description":"Echoes input","inputSchema":{"type":"object"}}]}}`+"\n",
		)
		// when
		err := c.RegisterTools(t.Context(), tb)
		// then
		require.NoError(t, err)
		registered := tb.Tools()
		require.Len(t, registered, 1)
		assert.Equal(t, "srv__echo", registered[0].Name)
		assert.Equal(t, "Echoes input", registered[0].Description)
		assert.Equal(t, map[string]any{"type": "object"}, registered[0].Schema)
	})

	t.Run("registers a handler that forwards the call and returns the text", func(t *testing.T) {
		// given
		c, tb, in := newMemClient("srv",
			`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"echo"}]}}`+"\n",
			`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"hello"},{"type":"text","text":"world"}]}}`+"\n",
		)
		require.NoError(t, c.RegisterTools(t.Context(), tb))
		// when
		result, err := tb.Execute(t.Context(), llm.ToolCall{Name: "srv__echo", Arguments: map[string]any{"city": "Lisbon"}})
		// then
		require.NoError(t, err)
		assert.Equal(t, "hello\nworld", result.Content)
		sent := sentMessages(t, in)
		require.Len(t, sent, 2)
		assert.Equal(t, "tools/call", sent[1]["method"])
		assert.Equal(t, map[string]any{"name": "echo", "arguments": map[string]any{"city": "Lisbon"}}, sent[1]["params"])
	})

	t.Run("a tools/call error result surfaces as a handler error", func(t *testing.T) {
		// given: the server marks the call result with isError
		c, tb, _ := newMemClient("srv",
			`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"echo"}]}}`+"\n",
			`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"boom"}],"isError":true}}`+"\n",
		)
		require.NoError(t, c.RegisterTools(t.Context(), tb))
		// when
		result, err := tb.Execute(t.Context(), llm.ToolCall{Name: "srv__echo"})
		// then
		assert.Nil(t, result)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "boom")
	})

	t.Run("registers nothing when the server returns no tools", func(t *testing.T) {
		// given
		c, tb, _ := newMemClient("srv", `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`+"\n")
		// when
		err := c.RegisterTools(t.Context(), tb)
		// then
		require.NoError(t, err)
		assert.Empty(t, tb.Tools())
	})

	t.Run("returns ErrAlreadyRegistered on a second call", func(t *testing.T) {
		// given
		c, tb, _ := newMemClient("srv", `{"jsonrpc":"2.0","id":1,"result":{"tools":[]}}`+"\n")
		require.NoError(t, c.RegisterTools(t.Context(), tb))
		// when
		err := c.RegisterTools(t.Context(), tools.NewToolBox())
		// then
		assert.ErrorIs(t, err, ErrAlreadyRegistered)
	})

	t.Run("a failed tools/list leaves the client registrable", func(t *testing.T) {
		// given: an empty stream, so tools/list fails with a transport error
		c, tb, _ := newMemClient("srv")
		// when: the first registration fails
		firstErr := c.RegisterTools(t.Context(), tb)
		// then: it surfaces the transport error, not a registration state
		require.ErrorIs(t, firstErr, ErrMCPConnectionClosed)
		// and: a retry is not wedged by ErrAlreadyRegistered
		secondErr := c.RegisterTools(t.Context(), tb)
		assert.NotErrorIs(t, secondErr, ErrAlreadyRegistered)
	})
}

func TestClientClose(t *testing.T) {
	t.Run("removes the client's tools from the toolbox", func(t *testing.T) {
		// given
		c, tb, _ := newMemClient("srv", `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"echo"}]}}`+"\n")
		require.NoError(t, c.RegisterTools(t.Context(), tb))
		require.Len(t, tb.Tools(), 1)
		// when
		err := c.Close()
		// then
		require.NoError(t, err)
		assert.Empty(t, tb.Tools())
	})

	t.Run("is safe to call more than once", func(t *testing.T) {
		// given: a client with its tools registered
		c, tb, _ := newMemClient("srv", `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"echo"}]}}`+"\n")
		require.NoError(t, c.RegisterTools(t.Context(), tb))
		require.NoError(t, c.Close())
		// when: Close is called a second time
		err := c.Close()
		// then: it is a clean no-op and the tools stay removed
		require.NoError(t, err)
		assert.Empty(t, tb.Tools())
	})
}

func TestParseToolResult(t *testing.T) {
	t.Run("joins the text parts with newlines and reports success", func(t *testing.T) {
		// given
		result := map[string]any{"content": []any{
			map[string]any{"type": "text", "text": "a"},
			map[string]any{"type": "image", "data": "..."},
			map[string]any{"type": "text", "text": "b"},
		}}
		// when
		text, failed := parseToolResult(result)
		// then
		assert.Equal(t, "a\nb", text)
		assert.False(t, failed)
	})

	t.Run("reports failed when the result is flagged with isError", func(t *testing.T) {
		// given
		result := map[string]any{"content": []any{
			map[string]any{"type": "text", "text": "boom"},
		}, "isError": true}
		// when
		text, failed := parseToolResult(result)
		// then
		assert.Equal(t, "boom", text)
		assert.True(t, failed)
	})

	t.Run("falls back to JSON when there is no text content", func(t *testing.T) {
		// given
		result := map[string]any{"isError": true}
		// when
		text, failed := parseToolResult(result)
		// then
		assert.JSONEq(t, `{"isError":true}`, text)
		assert.True(t, failed)
	})
}

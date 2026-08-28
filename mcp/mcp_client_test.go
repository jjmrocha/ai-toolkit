package mcp

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMemClient(name string, responses ...string) (*Client, *tools.ToolBox, *fakeTransport) {
	s, in := newMemSession(responses...)
	c := &Client{
		config:  ClientConfig{Name: name},
		session: s,
	}
	return c, tools.NewToolBox(), in
}

func endlessToolPages(count int) []string {
	pages := make([]string, 0, count)

	for id := 1; id <= count; id++ {
		page := fmt.Sprintf(
			`{"jsonrpc":"2.0","id":%d,"result":{"tools":[{"name":"tool%d"}],"nextCursor":"page%d"}}`,
			id, id, id+1,
		)
		pages = append(pages, page)
	}

	return pages
}

func TestClientLargeToolResult(t *testing.T) {
	t.Run("delivers a tool result larger than a megabyte", func(t *testing.T) {
		// given: a server whose tool result exceeds the old 1 MiB message ceiling
		const payloadBytes = 2 * 1024 * 1024

		initResp := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":%q}}`, protocolVersion)
		listResp := `{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"big","description":"Big","inputSchema":{"type":"object"}}]}}`
		script := fmt.Sprintf(
			`big=$(head -c %d /dev/zero | tr '\0' 'a')
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*) echo '%s';;
    *'"method":"tools/list"'*) echo '%s';;
    *'"method":"tools/call"'*) printf '{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"%%s"}]}}\n' "$big";;
  esac
done`,
			payloadBytes, initResp, listResp,
		)

		tb := tools.NewToolBox()
		client, err := NewClient(t.Context(), ClientConfig{Name: "srv", Command: "sh", Args: []string{"-c", script}})
		require.NoError(t, err)
		t.Cleanup(func() { _ = client.Close() })
		require.NoError(t, client.RegisterTools(t.Context(), tb))
		// when
		result, err := tb.Execute(t.Context(), llm.ToolCall{Name: "srv__big"})
		// then
		require.NoError(t, err)
		assert.Len(t, result.Content, payloadBytes)
	})
}

func silentToolServerCmd(name string, timeout time.Duration) ClientConfig {
	initResp := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":%q}}`, protocolVersion)
	listResp := `{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"slow","description":"Slow","inputSchema":{"type":"object"}}]}}`
	script := fmt.Sprintf(
		`while IFS= read -r line; do case "$line" in *'"method":"initialize"'*) echo '%s';; *'"method":"tools/list"'*) echo '%s';; esac; done`,
		initResp, listResp,
	)

	return ClientConfig{
		Name:            name,
		Command:         "sh",
		Args:            []string{"-c", script},
		ToolCallTimeout: timeout,
	}
}

func silentToolBox(t *testing.T, timeout time.Duration) *tools.ToolBox {
	t.Helper()

	tb := tools.NewToolBox()
	client, err := NewClient(t.Context(), silentToolServerCmd("srv", timeout))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	require.NoError(t, client.RegisterTools(t.Context(), tb))

	return tb
}

func TestClientToolCallTimeout(t *testing.T) {
	t.Run("gives up on a silent server once the configured timeout passes", func(t *testing.T) {
		// given
		tb := silentToolBox(t, 200*time.Millisecond)
		// when
		start := time.Now()
		_, err := tb.Execute(t.Context(), llm.ToolCall{Name: "srv__slow"})
		elapsed := time.Since(start)
		// then
		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.Less(t, elapsed, 5*time.Second)
	})

	timeouts := []struct {
		name  string
		input time.Duration
	}{
		{name: "unset falls back to the default", input: 0},
		{name: "negative falls back to the default", input: -time.Second},
	}

	for _, tc := range timeouts {
		t.Run(tc.name, func(t *testing.T) {
			// given: a timeout used as-is would expire the call at once
			tb := silentToolBox(t, tc.input)
			done := make(chan struct{})
			// when
			go func() {
				defer close(done)
				_, _ = tb.Execute(t.Context(), llm.ToolCall{Name: "srv__slow"})
			}()
			// then: still waiting, so the default is in force rather than the given value
			select {
			case <-done:
				t.Fatal("the call ended immediately; the configured value was used instead of the default")
			case <-time.After(500 * time.Millisecond):
			}
		})
	}
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

func TestClientConnected(t *testing.T) {
	t.Run("reports a client whose server is running", func(t *testing.T) {
		// given
		c := liveClient(t, "srv")
		// when
		result := c.Connected()
		// then
		assert.True(t, result)
	})

	t.Run("reports a client whose server is gone", func(t *testing.T) {
		// given
		c := deadClient(t, "srv")
		// when
		result := c.Connected()
		// then
		assert.False(t, result)
	})
}

func TestClientRegisterTools(t *testing.T) {
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

	t.Run("registers a handler that fails on a tools/call error result", func(t *testing.T) {
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

	t.Run("surfaces the transport error when tools/list fails", func(t *testing.T) {
		// given: an empty stream, so tools/list fails with a transport error
		c, tb, _ := newMemClient("srv")
		// when
		err := c.RegisterTools(t.Context(), tb)
		// then
		assert.ErrorIs(t, err, ErrMCPConnectionClosed)
		assert.Empty(t, tb.Tools())
	})

	t.Run("stays registrable after a failed tools/list", func(t *testing.T) {
		// given: a first registration that failed on the transport
		c, tb, _ := newMemClient("srv")
		require.ErrorIs(t, c.RegisterTools(t.Context(), tb), ErrMCPConnectionClosed)
		// when
		err := c.RegisterTools(t.Context(), tb)
		// then: the retry is not wedged by ErrAlreadyRegistered
		assert.NotErrorIs(t, err, ErrAlreadyRegistered)
	})

	t.Run("replaces the characters the providers reject", func(t *testing.T) {
		// given: a server whose tool name carries a dot
		c, tb, _ := newMemClient("srv",
			`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"github.create_issue"}]}}`+"\n",
		)
		// when
		err := c.RegisterTools(t.Context(), tb)
		// then: the tool is registered under a name the ToolBox accepts
		require.NoError(t, err)
		registered := tb.Tools()
		require.Len(t, registered, 1)
		assert.Equal(t, "srv__github_create_issue", registered[0].Name)
	})

	t.Run("calls the server with the name it published", func(t *testing.T) {
		// given: a tool registered under a sanitized name
		c, tb, in := newMemClient("srv",
			`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"github.create_issue"}]}}`+"\n",
			`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"done"}]}}`+"\n",
		)
		require.NoError(t, c.RegisterTools(t.Context(), tb))
		// when: the model calls it
		_, err := tb.Execute(t.Context(), llm.ToolCall{Name: "srv__github_create_issue"})
		// then: the server still sees the name it published
		require.NoError(t, err)
		sent := sentMessages(t, in)
		require.Len(t, sent, 2)
		params, ok := sent[1]["params"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "github.create_issue", params["name"])
	})

	t.Run("replaces a multi-byte character with a single underscore", func(t *testing.T) {
		// given: a name whose accented rune spans two bytes
		c, tb, _ := newMemClient("srv",
			`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"café"}]}}`+"\n",
		)
		// when
		err := c.RegisterTools(t.Context(), tb)
		// then: the result is pure ASCII, so a later truncation cannot split a rune
		require.NoError(t, err)
		registered := tb.Tools()
		require.Len(t, registered, 1)
		assert.Equal(t, "srv__caf_", registered[0].Name)
	})

	t.Run("truncates and tags a name that is too long", func(t *testing.T) {
		// given: a tool name that cannot fit the provider limit
		long := strings.Repeat("a", 100)
		c, tb, _ := newMemClient("srv",
			fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":%q}]}}`, long)+"\n",
		)
		// when
		err := c.RegisterTools(t.Context(), tb)
		// then: it fits exactly, and the tail marks it as derived
		require.NoError(t, err)
		registered := tb.Tools()
		require.Len(t, registered, 1)
		assert.Len(t, registered[0].Name, tools.MaxToolNameLength)
		assert.Regexp(t, `^srv__a+_[0-9a-f]{6}$`, registered[0].Name)
	})

	t.Run("keeps colliding names apart", func(t *testing.T) {
		// given: two names that sanitize to the same string
		c, tb, _ := newMemClient("srv",
			`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"a.b"},{"name":"a_b"}]}}`+"\n",
		)
		// when
		err := c.RegisterTools(t.Context(), tb)
		// then: neither tool shadows the other
		require.NoError(t, err)
		registered := tb.Tools()
		require.Len(t, registered, 2)
		assert.NotEqual(t, registered[0].Name, registered[1].Name)
	})

	t.Run("derives the same names on every registration", func(t *testing.T) {
		// given: the same server registered once already, into its own toolbox
		page := `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"a.b"},{"name":"a_b"}]}}` + "\n"
		first, firstBox, _ := newMemClient("srv", page)
		second, secondBox, _ := newMemClient("srv", page)
		require.NoError(t, first.RegisterTools(t.Context(), firstBox))
		// when
		err := second.RegisterTools(t.Context(), secondBox)
		// then: the generated names do not drift, so the prompt stays cacheable
		require.NoError(t, err)
		assert.Equal(t, firstBox.Tools(), secondBox.Tools())
	})

	t.Run("registers the tools of every page", func(t *testing.T) {
		// given: a server that splits its tools over two pages
		c, tb, _ := newMemClient("srv",
			`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"first"}],"nextCursor":"page2"}}`+"\n",
			`{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"second"}]}}`+"\n",
		)
		// when
		err := c.RegisterTools(t.Context(), tb)
		// then
		require.NoError(t, err)
		registered := tb.Tools()
		require.Len(t, registered, 2)
		assert.Equal(t, "srv__first", registered[0].Name)
		assert.Equal(t, "srv__second", registered[1].Name)
	})

	t.Run("asks for the next page with the cursor the server handed out", func(t *testing.T) {
		// given
		c, tb, in := newMemClient("srv",
			`{"jsonrpc":"2.0","id":1,"result":{"tools":[],"nextCursor":"page2"}}`+"\n",
			`{"jsonrpc":"2.0","id":2,"result":{"tools":[]}}`+"\n",
		)
		// when
		err := c.RegisterTools(t.Context(), tb)
		// then: the first request carries no cursor, the second carries the server's
		require.NoError(t, err)
		sent := sentMessages(t, in)
		require.Len(t, sent, 2)
		assert.Equal(t, "tools/list", sent[0]["method"])
		assert.Equal(t, map[string]any{}, sent[0]["params"])
		assert.Equal(t, "tools/list", sent[1]["method"])
		assert.Equal(t, map[string]any{"cursor": "page2"}, sent[1]["params"])
	})

	t.Run("stops at an empty nextCursor", func(t *testing.T) {
		// given: a server that answers with a cursor it left empty
		c, tb, in := newMemClient("srv",
			`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"only"}],"nextCursor":""}}`+"\n",
			`{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"never"}]}}`+"\n",
		)
		// when
		err := c.RegisterTools(t.Context(), tb)
		// then: the second page is never asked for
		require.NoError(t, err)
		assert.Len(t, tb.Tools(), 1)
		assert.Len(t, sentMessages(t, in), 1)
	})

	t.Run("registers nothing when a page fails", func(t *testing.T) {
		// given: the script ends before the second page, so the connection closes
		c, tb, _ := newMemClient("srv",
			`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"first"}],"nextCursor":"page2"}}`+"\n",
		)
		// when
		err := c.RegisterTools(t.Context(), tb)
		// then: the tools of the page that did arrive are not registered
		assert.ErrorIs(t, err, ErrMCPConnectionClosed)
		assert.Empty(t, tb.Tools())
	})

	t.Run("stays registrable after a failed page", func(t *testing.T) {
		// given: a first registration that lost the connection mid-pagination
		c, tb, _ := newMemClient("srv",
			`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"first"}],"nextCursor":"page2"}}`+"\n",
		)
		require.ErrorIs(t, c.RegisterTools(t.Context(), tb), ErrMCPConnectionClosed)
		// when
		err := c.RegisterTools(t.Context(), tb)
		// then: the retry is not wedged by ErrAlreadyRegistered
		assert.NotErrorIs(t, err, ErrAlreadyRegistered)
	})

	t.Run("returns ErrTooManyToolPages when the server never ends the list", func(t *testing.T) {
		// given: every page hands out another cursor
		c, tb, _ := newMemClient("srv", endlessToolPages(maxToolPages)...)
		// when
		err := c.RegisterTools(t.Context(), tb)
		// then
		require.ErrorIs(t, err, ErrTooManyToolPages)
		assert.Empty(t, tb.Tools())
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

	t.Run("does not queue behind a registration waiting on the server", func(t *testing.T) {
		// given: a server that takes the request and never answers
		ft := newFakeTransport()
		c := &Client{config: ClientConfig{Name: "srv"}, session: newTestSession(ft)}
		registered := make(chan error, 1)

		go func() { registered <- c.RegisterTools(t.Context(), tools.NewToolBox()) }()
		<-ft.written
		// when
		closed := make(chan error, 1)
		go func() { closed <- c.Close() }()
		// then
		select {
		case err := <-closed:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("Close blocked behind the in-flight registration")
		}
	})

	t.Run("aborts a registration that is waiting on the server", func(t *testing.T) {
		// given: a registration parked on a reply that will never come
		ft := newFakeTransport()
		c := &Client{config: ClientConfig{Name: "srv"}, session: newTestSession(ft)}
		registered := make(chan error, 1)

		go func() { registered <- c.RegisterTools(t.Context(), tools.NewToolBox()) }()
		<-ft.written
		// when
		require.NoError(t, c.Close())
		// then: the registration gives up instead of waiting out its deadline
		select {
		case err := <-registered:
			assert.ErrorIs(t, err, ErrMCPConnectionClosed)
		case <-time.After(5 * time.Second):
			t.Fatal("the registration outlived Close")
		}
	})

	t.Run("is safe to call more than once", func(t *testing.T) {
		// given: a client with its tools registered, already closed once
		c, tb, _ := newMemClient("srv", `{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"echo"}]}}`+"\n")
		require.NoError(t, c.RegisterTools(t.Context(), tb))
		require.NoError(t, c.Close())
		// when
		err := c.Close()
		// then: it is a clean no-op and the tools stay removed
		require.NoError(t, err)
		assert.Empty(t, tb.Tools())
	})
}

func TestParseToolResult(t *testing.T) {
	t.Run("joins the text parts with newlines and reports success", func(t *testing.T) {
		// given
		toolResult := map[string]any{"content": []any{
			map[string]any{"type": "text", "text": "a"},
			map[string]any{"type": "image", "data": "..."},
			map[string]any{"type": "text", "text": "b"},
		}}
		// when
		result, failed := parseToolResult(toolResult)
		// then
		assert.Equal(t, "a\nb", result)
		assert.False(t, failed)
	})

	t.Run("reports failed when the result is flagged with isError", func(t *testing.T) {
		// given
		toolResult := map[string]any{"content": []any{
			map[string]any{"type": "text", "text": "boom"},
		}, "isError": true}
		// when
		result, failed := parseToolResult(toolResult)
		// then
		assert.Equal(t, "boom", result)
		assert.True(t, failed)
	})

	t.Run("falls back to JSON when there is no text content", func(t *testing.T) {
		// given
		toolResult := map[string]any{"isError": true}
		// when
		result, failed := parseToolResult(toolResult)
		// then
		assert.JSONEq(t, `{"isError":true}`, result)
		assert.True(t, failed)
	})
}

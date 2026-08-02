package mcp

import (
	"fmt"
	"sync"
	"testing"

	"github.com/jjmrocha/ai-toolkit/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// liveClient wraps a still-open transport so Connected reports true.
func liveClient(t testing.TB, name string) *Client {
	t.Helper()
	return &Client{config: ClientConfig{Name: name}, transport: newTestStdio(newFakeTransport())}
}

// deadClient wraps a transport that has already gone away so Connected reports false.
func deadClient(t testing.TB, name string) *Client {
	t.Helper()

	ft := newFakeTransport()
	s := newTestStdio(ft)
	ft.Close()

	return &Client{config: ClientConfig{Name: name}, transport: s}
}

// scriptServerCmd returns a command acting as a minimal MCP server: it reads one
// request per line and answers initialize and tools/list, ignoring anything else.
// Responses are written only in reply to a request, because the client drops any
// response arriving before the matching request is in flight. The loop ends on
// stdin EOF, so Close stays prompt.
func scriptServerCmd(toolsResp string) ClientConfig {
	initResp := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":%q}}`, protocolVersion)
	script := fmt.Sprintf(
		`while IFS= read -r line; do case "$line" in *'"method":"initialize"'*) echo '%s';; *'"method":"tools/list"'*) echo '%s';; esac; done`,
		initResp, toolsResp,
	)

	return ClientConfig{Name: "srv", Command: "sh", Args: []string{"-c", script}}
}

// echoServerCmd answers tools/list with one usable tool.
func echoServerCmd() ClientConfig {
	return scriptServerCmd(`{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"echo","description":"Echoes","inputSchema":{"type":"object"}}]}}`)
}

// badToolServerCmd answers tools/list with a tool whose namespaced name is
// invalid, so RegisterTools fails while the process stays alive.
func badToolServerCmd() ClientConfig {
	return scriptServerCmd(`{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"bad name","description":"","inputSchema":{"type":"object"}}]}}`)
}

func TestManagerStart(t *testing.T) {
	t.Run("returns ErrMCPNotRegistered for an unknown name", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		// when
		err := m.Start(t.Context(), "missing")
		// then
		assert.ErrorIs(t, err, ErrMCPNotRegistered)
	})

	t.Run("propagates the error when the server fails to start", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		m.Register(ClientConfig{Name: "srv", Command: "definitely-not-a-real-command"})
		// when
		err := m.Start(t.Context(), "srv")
		// then
		require.Error(t, err)
		assert.NotContains(t, m.clients, "srv")
	})

	t.Run("rolls back the client when tool registration fails", func(t *testing.T) {
		// given: a server that starts fine but whose tools cannot be registered
		m := NewManager(tools.NewToolBox())
		m.Register(badToolServerCmd())
		t.Cleanup(m.Close)
		// when
		err := m.Start(t.Context(), "srv")
		// then: the error propagates and no broken client is left behind
		require.Error(t, err)
		assert.NotContains(t, m.clients, "srv")
		assert.Empty(t, m.toolBox.Tools())
	})

	t.Run("starts a registered MCP and registers its tools", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		m.Register(echoServerCmd())
		t.Cleanup(m.Close)
		// when
		err := m.Start(t.Context(), "srv")
		// then
		require.NoError(t, err)
		registered := m.toolBox.Tools()
		require.Len(t, registered, 1)
		assert.Equal(t, "srv__echo", registered[0].Name)
		assert.True(t, m.clients["srv"].Connected())
	})

	t.Run("reuses a client that is already running", func(t *testing.T) {
		// given: an MCP that has already been started
		m := NewManager(tools.NewToolBox())
		m.Register(echoServerCmd())
		t.Cleanup(m.Close)
		require.NoError(t, m.Start(t.Context(), "srv"))
		existing := m.clients["srv"]
		// when: Start is called again while it is still running
		err := m.Start(t.Context(), "srv")
		// then: the same client is kept and its tools are not registered twice
		require.NoError(t, err)
		assert.Same(t, existing, m.clients["srv"])
		assert.Len(t, m.toolBox.Tools(), 1)
	})

	t.Run("replaces a client whose process has died", func(t *testing.T) {
		// given: a registered MCP whose recorded client is already dead
		m := NewManager(tools.NewToolBox())
		m.Register(echoServerCmd())
		m.clients["srv"] = deadClient(t, "srv")
		t.Cleanup(m.Close)
		// when
		err := m.Start(t.Context(), "srv")
		// then: a fresh, connected client replaced the dead one and tools registered
		require.NoError(t, err)
		assert.True(t, m.clients["srv"].Connected())
		require.Len(t, m.toolBox.Tools(), 1)
		assert.Equal(t, "srv__echo", m.toolBox.Tools()[0].Name)
	})
}

func TestManagerStop(t *testing.T) {
	t.Run("returns ErrMCPNotRegistered for an unknown name", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		// when
		err := m.Stop("missing")
		// then
		assert.ErrorIs(t, err, ErrMCPNotRegistered)
	})

	t.Run("returns no error when registered but not started", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		m.Register(ClientConfig{Name: "srv", Command: "server"})
		// when
		err := m.Stop("srv")
		// then
		require.NoError(t, err)
	})

	t.Run("stops a running MCP and forgets it", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		m.Register(ClientConfig{Name: "srv", Command: "server"})
		m.clients["srv"] = liveClient(t, "srv")
		// when
		err := m.Stop("srv")
		// then
		require.NoError(t, err)
		assert.NotContains(t, m.clients, "srv")
		// and: the config stays registered so it can be started again
		assert.Contains(t, m.configs, "srv")
	})
}

func TestManagerGetMCPs(t *testing.T) {
	t.Run("reports each registered MCP with its active state", func(t *testing.T) {
		// given: one running, one registered but never started
		m := NewManager(tools.NewToolBox())
		m.Register(ClientConfig{Name: "up", Command: "server"})
		m.Register(ClientConfig{Name: "down", Command: "server"})
		m.clients["up"] = liveClient(t, "up")
		t.Cleanup(m.Close)
		// when
		statuses := m.Status()
		// then
		byName := map[string]bool{}
		for _, s := range statuses {
			byName[s.Name] = s.Active
		}
		assert.Equal(t, map[string]bool{"up": true, "down": false}, byName)
	})

	t.Run("reports a dead client as inactive and reaps it", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		m.Register(ClientConfig{Name: "srv", Command: "server"})
		m.clients["srv"] = deadClient(t, "srv")
		// when
		statuses := m.Status()
		// then: reported inactive and dropped, but the config stays for a later Start
		require.Len(t, statuses, 1)
		assert.False(t, statuses[0].Active)
		assert.NotContains(t, m.clients, "srv")
		assert.Contains(t, m.configs, "srv")
	})
}

func TestManagerConcurrentAccess(t *testing.T) {
	t.Run("serves concurrent status reads and lifecycle changes safely", func(t *testing.T) {
		// given: several registered, running MCPs (validated by the race detector)
		m := NewManager(tools.NewToolBox())
		for i := range 5 {
			name := fmt.Sprintf("srv%d", i)
			m.Register(ClientConfig{Name: name, Command: "server"})
			m.clients[name] = liveClient(t, name)
		}
		t.Cleanup(m.Close)
		// when: many goroutines read status and mutate the registry at once
		var wg sync.WaitGroup
		for i := range 20 {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				m.Status()
				m.Register(ClientConfig{Name: fmt.Sprintf("new%d", i), Command: "server"})
				_ = m.Stop(fmt.Sprintf("srv%d", i%5))
			}(i)
		}
		// then: no data race or panic
		wg.Wait()
	})
}

func TestManagerClose(t *testing.T) {
	t.Run("closes every client and clears the registry", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		m.Register(ClientConfig{Name: "srv", Command: "server"})
		m.clients["srv"] = liveClient(t, "srv")
		// when
		m.Close()
		// then
		assert.Empty(t, m.clients)
		assert.Empty(t, m.configs)
	})
}

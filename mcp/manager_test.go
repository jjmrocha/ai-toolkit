package mcp

import (
	"fmt"
	"sync"
	"testing"

	"github.com/jjmrocha/ai-toolkit/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func liveClient(t testing.TB, name string) *Client {
	t.Helper()
	return &Client{config: ClientConfig{Name: name}, session: newTestSession(newFakeTransport())}
}

func deadClient(t testing.TB, name string) *Client {
	t.Helper()

	ft := newFakeTransport()
	s := newTestSession(ft)
	ft.Close()

	return &Client{config: ClientConfig{Name: name}, session: s}
}

func activeByName(t testing.TB, m *Manager) map[string]bool {
	t.Helper()

	byName := make(map[string]bool)
	for _, status := range m.Status() {
		byName[status.Name] = status.Active
	}

	return byName
}

func scriptServerCmd(toolsResp string) ClientConfig {
	initResp := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":%q}}`, protocolVersion)
	script := fmt.Sprintf(
		`while IFS= read -r line; do case "$line" in *'"method":"initialize"'*) echo '%s';; *'"method":"tools/list"'*) echo '%s';; esac; done`,
		initResp, toolsResp,
	)

	return ClientConfig{Name: "srv", Command: "sh", Args: []string{"-c", script}}
}

func echoServerCmd() ClientConfig {
	return scriptServerCmd(`{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"echo","description":"Echoes","inputSchema":{"type":"object"}}]}}`)
}

func failingToolsServerCmd() ClientConfig {
	return scriptServerCmd(`{"jsonrpc":"2.0","id":2,"error":{"code":-32603,"message":"tools unavailable"}}`)
}

func TestNewManager(t *testing.T) {
	t.Run("returns a manager with nothing registered", func(t *testing.T) {
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
	t.Run("makes the MCP known but does not start it", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		// when
		m.Register(ClientConfig{Name: "srv", Command: "server"})
		// then
		expected := map[string]bool{"srv": false}
		assert.Equal(t, expected, activeByName(t, m))
	})

	t.Run("replaces the configuration of a name already registered", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		m.Register(ClientConfig{Name: "srv", Command: "first"})
		// when
		m.Register(ClientConfig{Name: "srv", Command: "second"})
		// then: the name is registered once, under the newest configuration
		expected := map[string]bool{"srv": false}
		assert.Equal(t, expected, activeByName(t, m))
	})
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
		expected := map[string]bool{"srv": false}
		assert.Equal(t, expected, activeByName(t, m))
	})

	t.Run("rolls back the client when tool registration fails", func(t *testing.T) {
		// given: a server that starts fine but whose tools cannot be listed
		tb := tools.NewToolBox()
		m := NewManager(tb)
		m.Register(failingToolsServerCmd())
		t.Cleanup(m.Close)
		// when
		err := m.Start(t.Context(), "srv")
		// then: the error propagates and no broken client is left behind
		require.Error(t, err)
		expected := map[string]bool{"srv": false}
		assert.Equal(t, expected, activeByName(t, m))
		assert.Empty(t, tb.Tools())
	})

	t.Run("starts a registered MCP and registers its tools", func(t *testing.T) {
		// given
		tb := tools.NewToolBox()
		m := NewManager(tb)
		m.Register(echoServerCmd())
		t.Cleanup(m.Close)
		// when
		err := m.Start(t.Context(), "srv")
		// then
		require.NoError(t, err)
		registered := tb.Tools()
		require.Len(t, registered, 1)
		assert.Equal(t, "srv__echo", registered[0].Name)
		expected := map[string]bool{"srv": true}
		assert.Equal(t, expected, activeByName(t, m))
	})

	t.Run("reuses a client that is already running", func(t *testing.T) {
		// given: an MCP that has already been started
		tb := tools.NewToolBox()
		m := NewManager(tb)
		m.Register(echoServerCmd())
		t.Cleanup(m.Close)
		require.NoError(t, m.Start(t.Context(), "srv"))
		existing := m.clients["srv"]
		// when: Start is called again while it is still running
		err := m.Start(t.Context(), "srv")
		// then: the same client is kept and its tools are not registered twice
		require.NoError(t, err)
		assert.Same(t, existing, m.clients["srv"])
		assert.Len(t, tb.Tools(), 1)
	})

	t.Run("replaces a client whose process has died", func(t *testing.T) {
		// given: a registered MCP whose recorded client is already dead
		tb := tools.NewToolBox()
		m := NewManager(tb)
		m.Register(echoServerCmd())
		m.clients["srv"] = deadClient(t, "srv")
		t.Cleanup(m.Close)
		// when
		err := m.Start(t.Context(), "srv")
		// then: a fresh, connected client replaced the dead one and tools registered
		require.NoError(t, err)
		expected := map[string]bool{"srv": true}
		assert.Equal(t, expected, activeByName(t, m))
		registered := tb.Tools()
		require.Len(t, registered, 1)
		assert.Equal(t, "srv__echo", registered[0].Name)
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

	t.Run("stops a running MCP and keeps it registered", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		m.Register(ClientConfig{Name: "srv", Command: "server"})
		m.clients["srv"] = liveClient(t, "srv")
		// when
		err := m.Stop("srv")
		// then: it stops running, and stays listed so it can be started again
		require.NoError(t, err)
		expected := map[string]bool{"srv": false}
		assert.Equal(t, expected, activeByName(t, m))
	})
}

func TestManagerStatus(t *testing.T) {
	t.Run("reports each registered MCP with its active state", func(t *testing.T) {
		// given: one running, one registered but never started
		m := NewManager(tools.NewToolBox())
		m.Register(ClientConfig{Name: "up", Command: "server"})
		m.Register(ClientConfig{Name: "down", Command: "server"})
		m.clients["up"] = liveClient(t, "up")
		t.Cleanup(m.Close)
		// when
		result := m.Status()
		// then
		byName := make(map[string]bool)
		for _, status := range result {
			byName[status.Name] = status.Active
		}

		expected := map[string]bool{"up": true, "down": false}
		assert.Equal(t, expected, byName)
	})

	t.Run("reports a dead client as inactive and reaps it", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		m.Register(ClientConfig{Name: "srv", Command: "server"})
		m.clients["srv"] = deadClient(t, "srv")
		// when
		result := m.Status()
		// then: reported inactive, and the config stays for a later Start
		require.Len(t, result, 1)
		assert.False(t, result[0].Active)
		expected := map[string]bool{"srv": false}
		assert.Equal(t, expected, activeByName(t, m))
	})

	t.Run("returns nothing when no MCP is registered", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		// when
		result := m.Status()
		// then
		assert.Empty(t, result)
	})
}

func TestManagerConcurrentAccess(t *testing.T) {
	// given: several registered, running MCPs (validated by the race detector)
	const goroutines = 20
	const servers = 5

	m := NewManager(tools.NewToolBox())
	for i := range servers {
		name := fmt.Sprintf("srv%d", i)
		m.Register(ClientConfig{Name: name, Command: "server"})
		m.clients[name] = liveClient(t, name)
	}

	t.Cleanup(m.Close)

	var wg sync.WaitGroup
	// when: many goroutines read status and mutate the registry at once
	for i := range goroutines {
		wg.Go(func() {
			m.Status()
			m.Register(ClientConfig{Name: fmt.Sprintf("new%d", i), Command: "server"})
			_ = m.Stop(fmt.Sprintf("srv%d", i%servers))
		})
	}

	wg.Wait()
	// then: every registration survived and every server was stopped
	result := activeByName(t, m)
	assert.Len(t, result, goroutines+servers)

	for name, active := range result {
		assert.False(t, active, name)
	}
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
		assert.Empty(t, m.Status())
	})
}

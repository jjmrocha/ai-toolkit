package mcp

import (
	"fmt"
	"sync"
	"testing"

	"github.com/jjmrocha/ai-toolkit/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// liveClient wraps a real, still-running child process so Connected reports true.
// cat blocks on stdin and exits on EOF, so close returns promptly.
func liveClient(t testing.TB, name string) *Client {
	t.Helper()
	return &Client{config: ClientConfig{Name: name}, transport: startProcess(t, "cat")}
}

// deadClient wraps a child process that has already exited so Connected reports false.
func deadClient(t testing.TB, name string) *Client {
	t.Helper()
	s := startProcess(t, "sh", "-c", "exit 0")
	<-s.exited
	return &Client{config: ClientConfig{Name: name}, transport: s}
}

// echoServerCmd returns a command that acts as a minimal MCP server: it answers
// the initialize handshake and a single tools/list, then stays alive.
func echoServerCmd() ClientConfig {
	initResp := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":%q}}`, protocolVersion)
	toolsResp := `{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"echo","description":"Echoes","inputSchema":{"type":"object"}}]}}`
	// After answering, drain stdin to stay alive and exit on EOF so Close is prompt.
	script := fmt.Sprintf("echo '%s'; echo '%s'; cat >/dev/null", initResp, toolsResp)

	return ClientConfig{Name: "srv", Command: "sh", Args: []string{"-c", script}}
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
		m.RegisterClient(ClientConfig{Name: "srv", Command: "definitely-not-a-real-command"})
		// when
		err := m.Start(t.Context(), "srv")
		// then
		require.Error(t, err)
		assert.NotContains(t, m.clients, "srv")
	})

	t.Run("starts a registered MCP and registers its tools", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		m.RegisterClient(echoServerCmd())
		t.Cleanup(m.Close)
		// when
		err := m.Start(t.Context(), "srv")
		// then
		require.NoError(t, err)
		registered := m.toolBox.GetTools()
		require.Len(t, registered, 1)
		assert.Equal(t, "srv__echo", registered[0].Name)
		assert.True(t, m.clients["srv"].Connected())
	})

	t.Run("replaces a client whose process has died", func(t *testing.T) {
		// given: a registered MCP whose recorded client is already dead
		m := NewManager(tools.NewToolBox())
		m.RegisterClient(echoServerCmd())
		m.clients["srv"] = deadClient(t, "srv")
		t.Cleanup(m.Close)
		// when
		err := m.Start(t.Context(), "srv")
		// then: a fresh, connected client replaced the dead one and tools registered
		require.NoError(t, err)
		assert.True(t, m.clients["srv"].Connected())
		require.Len(t, m.toolBox.GetTools(), 1)
		assert.Equal(t, "srv__echo", m.toolBox.GetTools()[0].Name)
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

	t.Run("returns ErrMCPNotRunning when registered but not started", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		m.RegisterClient(ClientConfig{Name: "srv", Command: "server"})
		// when
		err := m.Stop("srv")
		// then
		assert.ErrorIs(t, err, ErrMCPNotRunning)
	})

	t.Run("stops a running MCP and forgets it", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		m.RegisterClient(ClientConfig{Name: "srv", Command: "server"})
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
		m.RegisterClient(ClientConfig{Name: "up", Command: "server"})
		m.RegisterClient(ClientConfig{Name: "down", Command: "server"})
		m.clients["up"] = liveClient(t, "up")
		t.Cleanup(m.Close)
		// when
		statuses := m.GetMCPs()
		// then
		byName := map[string]bool{}
		for _, s := range statuses {
			byName[s.Name] = s.Active
		}
		assert.Equal(t, map[string]bool{"up": true, "down": false}, byName)
	})

	t.Run("reports a dead client as inactive without reaping it", func(t *testing.T) {
		// given
		m := NewManager(tools.NewToolBox())
		m.RegisterClient(ClientConfig{Name: "srv", Command: "server"})
		m.clients["srv"] = deadClient(t, "srv")
		// when
		statuses := m.GetMCPs()
		// then: read-only — reported inactive, but the entry stays for Start/Stop to own
		require.Len(t, statuses, 1)
		assert.False(t, statuses[0].Active)
		assert.Contains(t, m.clients, "srv")
	})
}

func TestManagerConcurrentAccess(t *testing.T) {
	t.Run("serves concurrent status reads and lifecycle changes safely", func(t *testing.T) {
		// given: several registered, running MCPs (validated by the race detector)
		m := NewManager(tools.NewToolBox())
		for i := range 5 {
			name := fmt.Sprintf("srv%d", i)
			m.RegisterClient(ClientConfig{Name: name, Command: "server"})
			m.clients[name] = liveClient(t, name)
		}
		t.Cleanup(m.Close)
		// when: many goroutines read status and mutate the registry at once
		var wg sync.WaitGroup
		for i := range 20 {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				m.GetMCPs()
				m.RegisterClient(ClientConfig{Name: fmt.Sprintf("new%d", i), Command: "server"})
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
		m.RegisterClient(ClientConfig{Name: "srv", Command: "server"})
		m.clients["srv"] = liveClient(t, "srv")
		// when
		m.Close()
		// then
		assert.Empty(t, m.clients)
		assert.Empty(t, m.configs)
	})
}

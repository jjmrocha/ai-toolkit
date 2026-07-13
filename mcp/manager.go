package mcp

import (
	"context"
	"sync"

	"github.com/jjmrocha/ai-toolkit/tools"
)

// Manager registers MCP servers by name and runs them on demand against a shared
// tools.ToolBox. Register a server with RegisterMCP, then Start and Stop it by
// name; GetMCPStatus reports which are running. It is safe for concurrent use.
type Manager struct {
	toolBox *tools.ToolBox
	configs map[string]ClientConfig
	clients map[string]*Client
	mu      sync.RWMutex
}

// NewManager returns an empty Manager that registers each MCP's tools into tb.
func NewManager(tb *tools.ToolBox) *Manager {
	return &Manager{
		toolBox: tb,
		configs: make(map[string]ClientConfig),
		clients: make(map[string]*Client),
	}
}

// Close stops every running MCP and clears the registry.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, client := range m.clients {
		delete(m.clients, name)
		delete(m.configs, name)
		_ = client.Close()
	}
}

// RegisterMCP adds an MCP's configuration under cfg.Name so it can
// later be started by name. Registering an existing name replaces its config.
// It does not start the server.
func (m *Manager) RegisterMCP(cfg ClientConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.configs[cfg.Name] = cfg
}

// GetMCPStatus reports the registered MCPs and whether each is currently running. A
// client whose process has died is reported inactive and reaped, so the next
// Start launches a fresh one. It takes the write lock because of that reaping.
func (m *Manager) GetMCPStatus() []Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	statuses := make([]Status, 0, len(m.configs))
	for name := range m.configs {
		active := false

		if client, ok := m.clients[name]; ok {
			active = client.Connected()

			if !active {
				// The process has died; drop it so the next Start will launch a fresh one.
				_ = client.Close()
				delete(m.clients, name)
			}
		}

		status := Status{Name: name, Active: active}
		statuses = append(statuses, status)
	}

	return statuses
}

// Start launches the MCP registered under name and registers its tools in the
// ToolBox. A client that is already running is reused; one whose process has
// died is discarded and replaced. It returns ErrMCPNotRegistered when no MCP is
// registered under name, or the underlying launch or registration error. ctx
// bounds the startup handshake and the tools/list request.
func (m *Manager) Start(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg, ok := m.configs[name]
	if !ok {
		return ErrMCPNotRegistered
	}

	if client, ok := m.clients[name]; ok {
		if client.Connected() {
			return nil
		}

		// The process has died; drop it and start a fresh one below.
		_ = client.Close()
		delete(m.clients, name)
	}

	client, err := NewClient(ctx, cfg)
	if err != nil {
		return err
	}

	if err := client.RegisterTools(ctx, m.toolBox); err != nil {
		// Registration failed; discard the client so it is not left behind
		// reporting itself as running with missing tools.
		_ = client.Close()
		return err
	}

	m.clients[name] = client

	return nil
}

// Stop shuts down the running MCP named name, removing its tools from the
// ToolBox, and keeps its configuration so it can be started again. It returns
// ErrMCPNotRegistered when no MCP is registered under name.
func (m *Manager) Stop(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.configs[name]; !ok {
		return ErrMCPNotRegistered
	}

	client, ok := m.clients[name]
	if !ok {
		return nil
	}

	_ = client.Close()
	delete(m.clients, name)

	return nil
}

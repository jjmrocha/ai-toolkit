package mcp

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/jjmrocha/ai-toolkit/tools"
)

// Manager registers MCP servers by name and runs them on demand against a
// shared [tools.ToolBox]. Register a server with [Manager.Register], then start
// and stop it by name with [Manager.Start] and [Manager.Stop];
// [Manager.Status] reports which are running. It is safe for concurrent use.
type Manager struct {
	toolBox *tools.ToolBox
	configs map[string]ClientConfig
	clients map[string]*Client
	mu      sync.Mutex
}

// NewManager returns an empty [Manager] that registers each MCP's tools into tb.
func NewManager(tb *tools.ToolBox) *Manager {
	return &Manager{
		toolBox: tb,
		configs: make(map[string]ClientConfig),
		clients: make(map[string]*Client),
	}
}

// Close stops every running MCP and removes its tools from the
// [tools.ToolBox]. Registrations are kept, so the same [Manager] can bring a
// server back up with [Manager.Start].
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, client := range m.clients {
		delete(m.clients, name)
		_ = client.Close()
	}
}

// Register adds an MCP's configuration under cfg.Name so it can
// later be started by name. Registering an existing name replaces its config.
// It does not start the server.
func (m *Manager) Register(cfg ClientConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.configs[cfg.Name] = cfg
}

// Status reports the registered MCPs and whether each is currently running. A
// client whose process has died is reported inactive and reaped, so the next
// [Manager.Start] launches a fresh one.
func (m *Manager) Status() []Status {
	m.mu.Lock()
	defer m.mu.Unlock()

	statuses := make([]Status, 0, len(m.configs))
	for name := range m.configs {
		active := false

		if client, ok := m.clients[name]; ok {
			active = client.Connected()

			if !active {
				_ = client.Close()
				delete(m.clients, name)
			}
		}

		status := Status{Name: name, Active: active}
		statuses = append(statuses, status)
	}

	return statuses
}

// Instructions returns the usage instructions of the running MCPs, paired
// with their registered names and sorted by name. An MCP that sent none is
// left out rather than returned empty, and one that is not running is left
// out too, its dead client reaped as [Manager.Status] does. It returns nil
// when no running MCP sent instructions.
func (m *Manager) Instructions() []Instruction {
	m.mu.Lock()
	defer m.mu.Unlock()

	instructions := make([]Instruction, 0, len(m.clients))

	for name, client := range m.clients {
		if !client.Connected() {
			_ = client.Close()
			delete(m.clients, name)

			continue
		}

		if instruction := client.Instructions(); instruction != nil {
			instructions = append(instructions, *instruction)
		}
	}

	if len(instructions) == 0 {
		return nil
	}

	slices.SortFunc(instructions, func(a, b Instruction) int {
		return strings.Compare(a.Name, b.Name)
	})

	return instructions
}

// Start launches the MCP registered under name and registers its tools in the
// [tools.ToolBox]. A client that is already running is reused; one whose process
// has died is discarded and replaced. It returns [ErrMCPNotRegistered] when no
// MCP is registered under name, or the underlying launch or registration error.
// ctx bounds the startup handshake and the tools/list request.
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

		_ = client.Close()
		delete(m.clients, name)
	}

	client, err := NewClient(ctx, cfg)
	if err != nil {
		return err
	}

	if err := client.RegisterTools(ctx, m.toolBox); err != nil {
		_ = client.Close()
		return err
	}

	m.clients[name] = client

	return nil
}

// Stop shuts down the running MCP named name, removing its tools from the
// [tools.ToolBox], and keeps its configuration so it can be started again. It
// returns [ErrMCPNotRegistered] when no MCP is registered under name.
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

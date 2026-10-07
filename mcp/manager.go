package mcp

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/jjmrocha/ai-toolkit/tools"
)

// Manager runs MCP servers by name against one [tools.ToolBox]. Add a server
// with [Manager.Register], start and stop it with [Manager.Start] and
// [Manager.Stop], and see which are running with [Manager.Status]. It is safe
// for concurrent use.
type Manager struct {
	toolBox *tools.ToolBox
	configs map[string]ClientConfig
	clients map[string]*Client
	mu      sync.Mutex
}

// NewManager returns an empty [Manager] that registers each MCP's tools in tb.
func NewManager(tb *tools.ToolBox) *Manager {
	return &Manager{
		toolBox: tb,
		configs: make(map[string]ClientConfig),
		clients: make(map[string]*Client),
	}
}

// Close stops every running MCP and removes its tools from the
// [tools.ToolBox]. Registrations are kept, so [Manager.Start] can start a
// server again.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, client := range m.clients {
		delete(m.clients, name)
		_ = client.Close()
	}
}

// Register stores cfg under cfg.Name, replacing any config with that name. It
// does not start the server.
func (m *Manager) Register(cfg ClientConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.configs[cfg.Name] = cfg
}

// Status returns each registered MCP and whether it is running. A client whose
// process has died is reported inactive and discarded, so the next
// [Manager.Start] launches a new one.
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

// Instructions returns the handshake instructions of the running MCPs, sorted
// by name. MCPs that sent none, or are not running, are left out; a dead
// client is discarded as in [Manager.Status]. It returns nil when there are
// none.
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
// [tools.ToolBox]. A running client is reused; a dead one is replaced. ctx
// bounds the handshake and the tools/list request. It returns
// [ErrMCPNotRegistered] when nothing is registered under name, or the launch or
// registration error.
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

	client, err := NewClient(ctx, cfg, m.toolBox)
	if err != nil {
		return err
	}

	m.clients[name] = client

	return nil
}

// Stop shuts down the MCP named name and removes its tools from the
// [tools.ToolBox]. Its config is kept, so it can be started again. It returns
// [ErrMCPNotRegistered] when nothing is registered under name.
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

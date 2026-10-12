package subagent

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/jjmrocha/ai-toolkit/agent"
	"github.com/jjmrocha/ai-toolkit/skills"
	"github.com/jjmrocha/ai-toolkit/tools"
	"github.com/jjmrocha/go-algo/fn"
)

type subAgent struct {
	config  Config
	agent   *agent.Agent
	toolBox *tools.ToolBox
	skills  *skills.Collection
}

// Manager holds the sub-agents a program offers, by name. Add one with
// [Manager.Register], list them with [Manager.Agents] and run a child of one
// with [Manager.Run]. It is safe for concurrent use, within the limit
// [Manager.Run] states.
type Manager struct {
	mu     sync.RWMutex
	agents map[string]subAgent
}

// NewManager returns an empty [Manager].
func NewManager() *Manager {
	return &Manager{
		agents: make(map[string]subAgent),
	}
}

// Register stores a sub-agent under cfg.Name, replacing any with that name.
//
// agent runs the tasks: every [Manager.Run] starts a new session on it and
// closes it afterwards. It brings its own model, iteration cap and
// [agent.Feedback]. It must not be the agent that asks for the sub-agent, whose
// conversation the run would replace.
//
// toolBox holds the tools the child may call; nil means no tools. Its
// [tools.Interceptor], if any, is the only one consulted for the child's calls.
// skills are the skills the child may load, registered in toolBox for the
// length of a run; nil or empty means no skills.
//
// Register returns [ErrNoName] when cfg.Name is empty and [ErrNoAgent] when
// agent is nil, registering nothing.
func (m *Manager) Register(cfg Config, agent *agent.Agent, toolBox *tools.ToolBox, skills *skills.Collection) error {
	if cfg.Name == "" {
		return ErrNoName
	}

	if agent == nil {
		return ErrNoAgent
	}

	registered := subAgent{
		config:  cfg,
		agent:   agent,
		toolBox: toolBox,
		skills:  skills,
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.agents[cfg.Name] = registered

	return nil
}

// Agents returns the name and description of every registered agent, sorted by
// name. It is empty when there are none.
func (m *Manager) Agents() []Info {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return fn.Map(m.names(), func(name string) Info {
		cfg := m.agents[name].config

		return Info{
			Name:        cfg.Name,
			Description: cfg.Description,
		}
	})
}

// Run gives prompt to the agent registered under name and returns its final
// answer, which may be empty. It starts a new session on the registered
// [agent.Agent] with the sub-agent's prompt, tools and skills, sends prompt as
// the first user message under ctx, and closes the session before returning.
// So the agent sees nothing but prompt, and no conversation outlives the call.
//
// Run returns [ErrNoPrompt] when prompt is empty and [ErrAgentNotFound] when no
// agent is registered under name, in both cases without calling the model. An
// error from [agent.Agent.Process], such as [agent.ErrMaxIterations], is
// returned unchanged.
//
// An [agent.Agent] is not safe for concurrent use, so two runs that use the
// same one must not overlap.
func (m *Manager) Run(ctx context.Context, name string, prompt string) (string, error) {
	if prompt == "" {
		return "", ErrNoPrompt
	}

	registered, err := m.lookup(name)
	if err != nil {
		return "", err
	}

	child := registered.agent
	defer child.Close()

	session := agent.SessionConfig{
		Prompt:  registered.config.Prompt,
		ToolBox: registered.toolBox,
		Skills:  registered.skills,
	}
	child.StartSession(session)

	response, err := child.Process(ctx, prompt)
	if err != nil {
		return "", err
	}

	return response.Content, nil
}

func (m *Manager) lookup(name string) (subAgent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	registered, ok := m.agents[name]
	if !ok {
		return subAgent{}, fmt.Errorf("%w: %q; registered agents: %s",
			ErrAgentNotFound, name, strings.Join(m.names(), ", "))
	}

	return registered, nil
}

func (m *Manager) names() []string {
	return slices.Sorted(maps.Keys(m.agents))
}

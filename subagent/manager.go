package subagent

import (
	"context"
	"fmt"
	"maps"
	"slices"
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

type childFeedback struct {
	agent.NopFeedback
	name string
	fb   Feedback
}

func (c *childFeedback) ToolCalled(toolName string, _ map[string]any) {
	c.fb.ToolCalled(c.name, toolName)
}

func (c *childFeedback) TokensUsed(totalTokens, cacheReadTokens int) {
	c.fb.TokensUsed(c.name, totalTokens, cacheReadTokens)
}

// Manager holds the sub-agents a program offers, by name. Add one with
// [Manager.Register], list them with [Manager.Agents] and run a child of one
// with [Manager.Run]. It is safe for concurrent use, within the limit
// [Manager.Run] states.
type Manager struct {
	mu     sync.RWMutex
	agents map[string]subAgent
	fb     Feedback
}

// NewManager returns an empty [Manager].
func NewManager() *Manager {
	return &Manager{
		agents: make(map[string]subAgent),
		fb:     NopFeedback{},
	}
}

// SetFeedback replaces the [Feedback] that receives the events of every run,
// for example with a chat UI's own. A nil fb is ignored. A run in progress keeps
// the Feedback it started with.
func (m *Manager) SetFeedback(fb Feedback) {
	if fb == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.fb = fb
}

// Register stores a sub-agent under cfg.Name, replacing any with that name.
//
// agent runs the tasks: every [Manager.Run] starts a new session on it and
// closes it afterwards. It brings its own model and iteration cap. [Manager.Run]
// replaces its [agent.Feedback] with one that reports to the manager's
// [Feedback]. It must not be the agent that asks for the sub-agent, whose
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
// Run reports the run's events to the manager's [Feedback]; see
// [Manager.SetFeedback].
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

	m.mu.RLock()
	fb := m.fb
	registered, ok := m.agents[name]
	m.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("subagent: %w: %s", ErrAgentNotFound, name)
	}

	child := registered.agent
	defer child.Close()

	child.SetFeedback(&childFeedback{name: name, fb: fb})
	fb.SubagentStarted(name)

	session := agent.SessionConfig{
		Prompt:  registered.config.Prompt,
		ToolBox: registered.toolBox,
		Skills:  registered.skills,
	}
	child.StartSession(session)

	response, err := child.Process(ctx, prompt)
	if err != nil {
		fb.SubagentFinished(name, err)
		return "", err
	}

	fb.TokensUsed(name, response.Metadata.TotalTokens, response.Metadata.CacheReadTokens)
	fb.SubagentFinished(name, nil)

	return response.Content, nil
}

func (m *Manager) names() []string {
	return slices.Sorted(maps.Keys(m.agents))
}

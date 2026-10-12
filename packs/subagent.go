package packs

import (
	"context"
	"strings"
	"sync"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/mcp"
	"github.com/jjmrocha/ai-toolkit/subagent"
	"github.com/jjmrocha/ai-toolkit/tools"
	"github.com/jjmrocha/go-algo/fn"
)

const (
	subagentToolName = "subagent"
	agentArg         = "agent"
	promptArg        = "prompt"
	subagentNoText   = "Subagent completed without a text response."
)

const subagentDescription = "Hand a self-contained task to a subagent and get back its final answer. " +
	"The subagent starts with an empty conversation and sees nothing of this one, so the prompt must " +
	"carry everything it needs: the goal, the facts and paths that matter, and what to report back. " +
	"The call blocks until the subagent finishes, and returns only its final answer, not the steps it " +
	"took. Pick the subagent whose description fits the task."

type subagentPack struct {
	toolBox *tools.ToolBox
	names   []string
	once    sync.Once
}

func (p *subagentPack) Close() error {
	p.once.Do(func() {
		for _, name := range p.names {
			p.toolBox.Remove(name)
		}
	})

	return nil
}

func (p *subagentPack) Instructions(_ context.Context) *mcp.Instruction {
	return nil
}

// SubAgentTools registers one tool in m, "subagent", that hands a task to one of
// the agents in manager and returns its final answer. The tool takes the
// agent's name and the prompt, and runs them through [subagent.Manager.Run], so
// the call blocks until the child answers. A child that ends without text
// returns a sentence saying so.
//
// The tool's description lists the agents registered in manager when
// SubAgentTools is called, by name and description, and is not updated
// afterwards. Register every agent first; to change the set, close the pack and
// call SubAgentTools again. It returns [ErrNoAgents] when manager has none.
//
// Give no agent m as its ToolBox: its children would see this tool and could
// start children of their own with no limit.
//
// The caller owns manager, so [ToolPack.Close] only unregisters. If m rejects
// the registration, SubAgentTools returns the error from [tools.ToolBox.Add].
func SubAgentTools(m *tools.ToolBox, manager *subagent.Manager) (ToolPack, error) {
	agents := manager.Agents()
	if len(agents) == 0 {
		return nil, ErrNoAgents
	}

	subagentTool := llm.Tool{
		Name:        subagentToolName,
		Description: subagentDescription + "\n\nAvailable subagents:\n" + renderAgents(agents),
		Schema: tools.NewObjectBuilder().
			String(agentArg, "the name of the subagent to run, from the available subagents", true).
			String(promptArg, "the task for the subagent, with all the context it needs", true).
			Build(),
	}

	subagentRegistration := registration{
		tool: subagentTool,
		handler: func(ctx context.Context, args map[string]any) (string, error) {
			return runSubAgent(ctx, manager, args)
		},
	}

	names, err := register(m, []registration{subagentRegistration})
	if err != nil {
		return nil, err
	}

	return &subagentPack{toolBox: m, names: names}, nil
}

func renderAgents(agents []subagent.Info) string {
	lines := fn.Map(agents, func(agent subagent.Info) string {
		if agent.Description == "" {
			return "- " + agent.Name
		}

		return "- " + agent.Name + ": " + agent.Description
	})

	return strings.Join(lines, "\n")
}

func runSubAgent(ctx context.Context, manager *subagent.Manager, args map[string]any) (string, error) {
	arguments := tools.NewArguments(args)

	name, err := arguments.GetString(agentArg)
	if err != nil {
		return "", err
	}

	prompt, err := arguments.GetString(promptArg)
	if err != nil {
		return "", err
	}

	answer, err := manager.Run(ctx, name, prompt)
	if err != nil {
		return "", err
	}

	if answer == "" {
		return subagentNoText, nil
	}

	return answer, nil
}

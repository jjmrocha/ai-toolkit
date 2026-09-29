package packs

import (
	"context"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/mcp"
	"github.com/jjmrocha/ai-toolkit/tools"
)

// ToolPack owns the tools one constructor registered, and whatever serves them.
type ToolPack interface {
	// Close removes the pack's tools and stops the server behind them, if any.
	// It is safe to call more than once. A pack backed by its own process that is
	// dropped without Close leaves that process running until the program exits.
	Close() error
	// Instructions returns usage guidance for the model, labeled by
	// [mcp.Instruction.Name], or nil if the pack has none. It complements the tool
	// descriptions. A pack served by an MCP server may ask the server under ctx
	// and return its error, so call it before [ToolPack.Close]. The packs that
	// serve their own tools return fixed text and never fail.
	Instructions(context.Context) (*mcp.Instruction, error)
}

type registration struct {
	tool    llm.Tool
	handler tools.Handler
}

func register(m *tools.ToolBox, registrations []registration) ([]string, error) {
	names := make([]string, 0, len(registrations))

	for _, r := range registrations {
		if err := m.Add(r.tool, r.handler); err != nil {
			return nil, err
		}

		names = append(names, r.tool.Name)
	}

	return names, nil
}

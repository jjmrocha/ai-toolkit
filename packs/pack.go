package packs

import (
	"context"

	"github.com/jjmrocha/ai-toolkit/mcp"
)

// ToolPack owns the tools one call registered in a ToolBox, and whatever serves
// them.
type ToolPack interface {
	// Close removes the tools the pack registered and stops whatever serves
	// them. A pack served by a process of its own leaves that process running
	// for the life of the program when it is dropped rather than closed, since
	// nothing else owns it. It is safe to call more than once.
	Close() error
	// Instructions returns the pack's usage doctrine — text meant for the
	// model using the tools, labeled by [mcp.Instruction.Name] and
	// complementary to the tool descriptions in the tools list — or nil when
	// the pack has none. A pack served by an MCP server may ask that server
	// for it under ctx, returning the error when the server fails to answer, so
	// call it before [ToolPack.Close]. A pack that serves its own tools returns
	// a fixed text and never fails.
	Instructions(context.Context) (*mcp.Instruction, error)
}

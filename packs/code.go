package packs

import (
	"context"
	"time"

	"github.com/jjmrocha/ai-toolkit/mcp"
	"github.com/jjmrocha/ai-toolkit/tools"
)

// SerenaMCPConfig returns the [mcp.ClientConfig] that [CodingTools] starts
// Serena from, in Serena's desktop-app context with its query-projects mode
// added. Every call returns a fresh value that shares nothing with the pack, so
// the returned config can be adjusted — pinned to a revision, say — and passed
// to [mcp.NewClient] directly.
func SerenaMCPConfig() mcp.ClientConfig {
	return mcp.ClientConfig{
		Name:    "serena",
		Command: "uvx",
		Args: []string{
			"--from", "git+https://github.com/oraios/serena",
			"serena", "start-mcp-server",
			"--context", "desktop-app",
			"--add-mode", "query-projects",
		},
		ToolCallTimeout: 360 * time.Second,
	}
}

// CodingTools registers symbol-aware code navigation and editing, diagnostics,
// file and directory access, shell execution, project memories and read-only
// queries against other projects in m, served by Serena
// (https://github.com/oraios/serena). It needs the uvx executable on PATH and
// no API key. The tools are registered under a "serena__" prefix, and the
// returned [ToolPack] removes them again.
//
// The server starts with no project, so the model works on a code base only
// after calling "serena__activate_project". Serena's own manual, which explains
// how its tools fit together, is a tool call away as
// "serena__initial_instructions".
//
// One project is active at a time, and activating another shuts the previous
// one's language servers down. To read a second code base without switching,
// the model calls "serena__query_project", which runs one read-only tool
// against a project Serena already has registered — editing tools and the shell
// are refused there. Its symbolic tools need Serena's project server running
// alongside the pack; the file and search tools do not.
//
// A registration that fails stops the server before returning, leaving nothing
// behind. A server that later dies on its own also removes its own tools from
// m, so a pack whose server is gone leaves no tool the model can still call.
func CodingTools(ctx context.Context, m *tools.ToolBox) (ToolPack, error) {
	client, err := mcp.NewClient(ctx, SerenaMCPConfig())
	if err != nil {
		return nil, err
	}

	err = client.RegisterTools(ctx, m)
	if err != nil {
		_ = client.Close()
		return nil, err
	}

	return client, nil
}

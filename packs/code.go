package packs

import (
	"context"
	"errors"
	"time"

	"github.com/jjmrocha/ai-toolkit/mcp"
	"github.com/jjmrocha/ai-toolkit/tools"
)

type codingPack struct {
	shell  ToolPack
	serena ToolPack
}

func (p *codingPack) Close() error {
	return errors.Join(p.shell.Close(), p.serena.Close())
}

// SerenaMCPConfig returns the [mcp.ClientConfig] that [CodingTools] starts
// Serena from, in Serena's desktop-app context with its query-projects mode
// added and its shell tool left unregistered. Every call returns a fresh value
// that shares nothing with the pack, so the returned config can be adjusted —
// pinned to a revision, say, or given its shell back — and passed to
// [mcp.NewClient] directly.
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
		ExcludedTools:   []string{"execute_shell_command"},
		ToolCallTimeout: 360 * time.Second,
	}
}

// CodingTools registers symbol-aware code navigation and editing, diagnostics,
// file and directory access, project memories and read-only queries against
// other projects in m, served by Serena (https://github.com/oraios/serena). It
// needs the uvx executable on PATH and no API key. The tools are registered
// under a "serena__" prefix, and the returned [ToolPack] removes them again.
//
// It also registers "shell_run", on the terms [ShellTools] gives it, in place
// of Serena's own shell tool, which is left unregistered. A code base is
// therefore navigated, edited and built from one pack, with the timeout and
// output ceiling "shell_run" applies rather than Serena's. Registering
// [ShellTools] on m as well is redundant, and closing either pack then takes
// "shell_run" from both.
//
// The server starts with no project, so the model works on a code base only
// after calling "serena__activate_project". Serena's own manual, which explains
// how its tools fit together, is a tool call away as
// "serena__initial_instructions".
//
// One project is active at a time, and activating another shuts the previous
// one's language servers down. To read a second code base without switching,
// the model calls "serena__query_project", which runs one read-only tool
// against a project Serena already has registered — editing tools are refused
// there. Its symbolic tools need Serena's project server running
// alongside the pack; the file and search tools do not.
//
// A registration that fails stops the server before returning, leaving nothing
// behind: "shell_run" is registered only once Serena's tools are in m. A server
// that later dies on its own removes its own tools from m, leaving "shell_run"
// registered until the pack is closed.
func CodingTools(ctx context.Context, m *tools.ToolBox) (ToolPack, error) {
	serena, err := serenaTools(ctx, m)
	if err != nil {
		return nil, err
	}

	shell, err := ShellTools(m)
	if err != nil {
		_ = serena.Close()

		return nil, err
	}

	return &codingPack{
		shell:  shell,
		serena: serena,
	}, nil
}

func serenaTools(ctx context.Context, m *tools.ToolBox) (ToolPack, error) {
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

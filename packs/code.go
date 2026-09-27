package packs

import (
	"context"
	"time"

	"github.com/jjmrocha/ai-toolkit/mcp"
	"github.com/jjmrocha/ai-toolkit/tools"
)

const serenaManualTool = "initial_instructions"

// SerenaMCPConfig returns the [mcp.ClientConfig] that [CodingTools] starts
// Serena from, in Serena's desktop-app context with its query-projects and
// no-memories modes added and its shell, memory and onboarding tools left
// unregistered. A non-empty project, a project name or path as Serena knows
// them, activates that project at startup; an empty one starts the server
// with none. Every call returns a fresh value that shares nothing with the
// pack, so the returned config can be adjusted — pinned to a revision, say,
// or given its shell back — and passed to [mcp.NewClient] directly.
func SerenaMCPConfig(project string) mcp.ClientConfig {
	args := []string{
		"--from", "git+https://github.com/oraios/serena",
		"serena", "start-mcp-server",
		"--context", "desktop-app",
		"--add-mode", "query-projects",
		"--add-mode", "no-memories",
	}
	if project != "" {
		args = append(args, "--project", project)
	}

	return mcp.ClientConfig{
		Name:    "serena",
		Command: "uvx",
		Args:    args,
		ExcludedTools: []string{
			"execute_shell_command",
			"write_memory", "read_memory", "list_memories",
			"edit_memory", "delete_memory", "rename_memory",
			"onboarding",
		},
		ToolCallTimeout: 360 * time.Second,
	}
}

type codingTools struct {
	mcp *mcp.Client
}

// CodingTools registers symbol-aware code navigation and editing, diagnostics,
// file and directory access and read-only queries against other projects in
// m, served by Serena (https://github.com/oraios/serena). It needs the uvx
// executable on PATH and no API key. The tools are registered under a
// "serena__" prefix, and the returned [ToolPack] removes them again.
//
// The pack gives the model no shell: Serena's own shell tool is left
// unregistered, and nothing takes its place. A model that has to build or run
// what it wrote needs [ShellTools] on m as well, which brings "shell_run" under
// a pack of its own.
//
// A non-empty project activates that project at startup, so the model works
// on that code base from the first symbolic call. An empty one leaves the
// server with no project, so the model reaches a code base only after
// calling "serena__activate_project", which still switches the active project
// either way.
//
// The pack's [ToolPack.Instructions] returns Serena's own manual, which
// explains how its tools fit together: each call asks Serena for it through
// its "initial_instructions" tool, so a model given that text does not have to
// call "serena__initial_instructions" itself. Serena's handshake instructions
// are only a line telling the model to make that call; Instructions falls back
// to them when the server fails to answer, and never returns an error.
//
// One project is active at a time, and activating another shuts the previous
// one's language servers down. To read a second code base without switching,
// the model calls "serena__query_project", which runs one read-only tool
// against a project Serena already has registered — editing tools are refused
// there. Its symbolic tools need Serena's project server running
// alongside the pack; the file and search tools do not.
//
// A registration that fails stops the server before returning, leaving nothing
// behind. A server that later dies on its own removes its own tools from m.
func CodingTools(ctx context.Context, m *tools.ToolBox, project string) (ToolPack, error) {
	cfg := SerenaMCPConfig(project)

	client, err := mcp.NewClient(ctx, cfg)
	if err != nil {
		return nil, err
	}

	err = client.RegisterTools(ctx, m)
	if err != nil {
		_ = client.Close()

		return nil, err
	}

	return &codingTools{mcp: client}, nil
}

func (c *codingTools) Instructions(ctx context.Context) (*mcp.Instruction, error) {
	manual, err := c.mcp.CallTool(ctx, serenaManualTool, nil)
	if err != nil {
		return c.mcp.Instructions(), nil
	}

	return &mcp.Instruction{
		Name: c.mcp.Name(),
		Text: manual,
	}, nil
}

func (c *codingTools) Close() error {
	return c.mcp.Close()
}

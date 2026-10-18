package packs

import (
	"context"
	"time"

	"github.com/jjmrocha/ai-toolkit/mcp"
	"github.com/jjmrocha/ai-toolkit/tools"
)

const serenaManualTool = "initial_instructions"

func serenaMCPConfig(project string, readOnly bool) mcp.ClientConfig {
	args := []string{
		"--from", "git+https://github.com/oraios/serena",
		"serena", "start-mcp-server",
		"--context", "desktop-app",
		"--add-mode", "query-projects",
		"--add-mode", "no-memories",
		"--project", project,
	}

	cfg := mcp.ClientConfig{
		Name:    "serena",
		Command: "uvx",
		Args:    args,
		ExcludedTools: []string{
			"execute_shell_command",
			"write_memory", "read_memory", "list_memories",
			"edit_memory", "delete_memory", "rename_memory",
			"onboarding",
			"activate_project", "get_current_config",
		},
		ToolCallTimeout: 360 * time.Second,
	}
	if readOnly {
		cfg.IncludedTools = []string{
			"read_file", "list_dir", "find_file", "search_for_pattern",
			"get_symbols_overview", "find_symbol", "find_referencing_symbols",
			"find_implementations", "find_declaration", "get_diagnostics_for_file",
			"list_queryable_projects", "query_project",
			"initial_instructions",
		}
	}

	return cfg
}

type codingTools struct {
	mcp *mcp.Client
}

// CodingTools registers the tools of Serena (https://github.com/oraios/serena)
// in m under a "serena__" prefix: symbol-aware navigation and editing,
// diagnostics, file access and read-only queries against other projects. It
// needs the uvx executable on PATH and no API key. Serena is installed from its
// repository's default branch on every launch, unpinned.
//
// There is no shell. Serena's own is excluded, so a model that builds or runs
// code also needs [ShellTools].
//
// With readOnly, only these tools are registered: "read_file", "list_dir",
// "find_file", "search_for_pattern", "get_symbols_overview", "find_symbol",
// "find_referencing_symbols", "find_implementations", "find_declaration",
// "get_diagnostics_for_file", "list_queryable_projects", "query_project" and
// "initial_instructions". A tool Serena adds later stays out, so an unpinned
// launch cannot bring in a new editing tool. "serena__query_project" stays, and
// Serena runs only its read tools through it.
//
// project is activated at startup and stays the active project; the model
// cannot switch to another. It returns [ErrNoProject] when project is empty.
// "serena__query_project" runs a read-only tool against another project Serena
// knows; its symbolic tools need Serena's project server running, its file and
// search tools do not.
//
// [ToolPack.Instructions] returns Serena's manual, fetched from its
// "initial_instructions" tool on every call, so the model does not have to call
// that tool itself. If Serena fails to answer, it returns the handshake
// instructions instead.
//
// If registration fails, the server is stopped before CodingTools returns. If
// the server dies later, its tools are removed from m.
func CodingTools(ctx context.Context, m *tools.ToolBox, project string, readOnly bool) (ToolPack, error) {
	if project == "" {
		return nil, ErrNoProject
	}

	cfg := serenaMCPConfig(project, readOnly)

	client, err := mcp.NewClient(ctx, cfg, m)
	if err != nil {
		return nil, err
	}

	return &codingTools{mcp: client}, nil
}

func (c *codingTools) Instructions(ctx context.Context) *mcp.Instruction {
	manual, err := c.mcp.CallTool(ctx, serenaManualTool, nil)
	if err != nil {
		return c.mcp.Instructions()
	}

	return &mcp.Instruction{
		Name: c.mcp.Name(),
		Text: manual,
	}
}

func (c *codingTools) Close() error {
	return c.mcp.Close()
}

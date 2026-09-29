package packs

import (
	"context"
	"time"

	"github.com/jjmrocha/ai-toolkit/mcp"
	"github.com/jjmrocha/ai-toolkit/tools"
)

const serenaManualTool = "initial_instructions"

// SerenaMCPConfig returns the [mcp.ClientConfig] [CodingTools] starts Serena
// with: the desktop-app context, the query-projects and no-memories modes, and
// Serena's shell, memory and onboarding tools excluded. A non-empty project (a
// name or path Serena knows) is activated at startup. Each call returns a new
// value, so it can be changed (to pin a revision or restore the shell, say) and
// passed to [mcp.NewClient] directly.
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

// CodingTools registers the tools of Serena (https://github.com/oraios/serena)
// in m under a "serena__" prefix: symbol-aware navigation and editing,
// diagnostics, file access and read-only queries against other projects. It
// needs the uvx executable on PATH and no API key.
//
// There is no shell. Serena's own is excluded, so a model that builds or runs
// code also needs [ShellTools].
//
// A non-empty project is activated at startup. With an empty one, the model has
// to call "serena__activate_project" before it can reach any code. One project
// is active at a time, and activating another stops the previous one's language
// servers. "serena__query_project" runs a read-only tool against another
// project Serena knows, without switching; its symbolic tools need Serena's
// project server running, its file and search tools do not.
//
// [ToolPack.Instructions] returns Serena's manual, fetched from its
// "initial_instructions" tool on every call, so the model does not have to call
// that tool itself. If Serena fails to answer, it returns the handshake
// instructions instead, and never an error.
//
// If registration fails, the server is stopped before CodingTools returns. If
// the server dies later, its tools are removed from m.
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

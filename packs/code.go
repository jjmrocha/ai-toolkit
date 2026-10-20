package packs

import (
	"context"
	"fmt"
	"time"

	"github.com/jjmrocha/ai-toolkit/mcp"
	"github.com/jjmrocha/ai-toolkit/tools"
)

const serenaInstruction = `These tools read the code of the project %[1]s by
symbol: functions, types, methods and fields. Paths are relative to the
project's root.

Serena counts lines from 0: the first line of a file is line 0, not line 1.
Every line number Serena reports or takes counts from 0: in the results of
serena__search_for_pattern and serena__find_referencing_symbols, in the
body_location of serena__find_symbol, and in the start_line and end_line of
serena__read_file and serena__get_diagnostics_for_file. Editors, compilers, test
output and other tools count from 1. Subtract 1 from such a number before you
pass it to Serena, and add 1 to a Serena number before you show it to the user
or pass it to another tool. A wrong number gives no error: Serena reads the line
next to the one you meant.

serena__read_file returns the text without line numbers. Never count lines in it
to find a line number. Take the number from serena__search_for_pattern,
serena__find_symbol or serena__find_referencing_symbols instead.

Read no more than the task needs. Start with serena__get_symbols_overview for a
file or serena__find_symbol for a name, and read a symbol's body only when you
need it. A symbol is named by its name path, such as "Type/Method", together
with its file. Use serena__find_referencing_symbols to see where a symbol is
used, and serena__search_for_pattern for text that is not a symbol name. Once
you have read a whole file, do not query its symbols again.

serena__query_project runs a read tool against another project that
serena__list_queryable_projects names, without leaving %[1]s. Its file and
search tools always work. Its symbolic tools need Serena's project server for
that project, which may not be running. When such a call fails, say so and fall
back to serena__search_for_pattern.

Serena runs calls one at a time, so independent calls can go out together in
one turn.`

const serenaEditInstruction = `Edit by symbol when the change covers a whole
symbol: serena__replace_symbol_body replaces a definition you have read with
include_body, and serena__insert_before_symbol and serena__insert_after_symbol
add code next to one. For a few lines inside a larger symbol, use
serena__replace_content on that file, in literal mode, or in regex mode with a
short pattern such as "start.*?end" instead of quoting the whole span. For the
same change in many places, use serena__replace_in_files: when a pattern could
match more than you mean, run it with dry_run first, then apply only the
occurrence ids you want. serena__create_text_file replaces any file that already
exists, so use it for new files only.

Rename with serena__rename_symbol and delete with serena__safe_delete_symbol:
both follow references across the project. serena__safe_delete_symbol deletes
nothing while the symbol is still used, and returns the references instead.
Before you change a symbol's signature, find its callers with
serena__find_referencing_symbols and update them in the same task.

An editing tool that returns without error has applied the change, so you do not
need to read the file again to check it.`

const serenaVersion = "2.1.0"

const serenaName = "serena"

func serenaMCPConfig(project string, readOnly bool) mcp.ClientConfig {
	args := []string{
		"--from", "serena-agent==" + serenaVersion,
		"serena", "start-mcp-server",
		"--context", "desktop-app",
		"--add-mode", "query-projects",
		"--add-mode", "no-memories",
		"--project", project,
	}

	cfg := mcp.ClientConfig{
		Name:    serenaName,
		Command: "uvx",
		Args:    args,
		ExcludedTools: []string{
			"execute_shell_command",
			"write_memory", "read_memory", "list_memories",
			"edit_memory", "delete_memory", "rename_memory",
			"onboarding",
			"activate_project", "get_current_config",
			"initial_instructions",
		},
		ToolCallTimeout: 360 * time.Second,
	}
	if readOnly {
		cfg.IncludedTools = []string{
			"read_file", "list_dir", "find_file", "search_for_pattern",
			"get_symbols_overview", "find_symbol", "find_referencing_symbols",
			"find_implementations", "find_declaration", "get_diagnostics_for_file",
			"list_queryable_projects", "query_project",
		}
	}

	return cfg
}

type codingTools struct {
	mcp      *mcp.Client
	project  string
	readOnly bool
}

// CodingTools registers the tools of Serena (https://github.com/oraios/serena)
// in m under a "serena__" prefix: symbol-aware navigation and editing,
// diagnostics, file access and read-only queries against other projects. It
// needs the uvx executable on PATH and no API key. uvx runs a fixed release of
// Serena, not the latest.
//
// There is no shell. Serena's own is excluded, so a model that builds or runs
// code also needs [ShellTools].
//
// With readOnly, only these tools are registered: "read_file", "list_dir",
// "find_file", "search_for_pattern", "get_symbols_overview", "find_symbol",
// "find_referencing_symbols", "find_implementations", "find_declaration",
// "get_diagnostics_for_file", "list_queryable_projects" and "query_project". A
// tool Serena adds later stays out, so a newer Serena release cannot bring in a
// new editing tool. "serena__query_project" stays, and Serena runs only its read
// tools through it.
//
// project is activated at startup and stays the active project; the model
// cannot switch to another. It returns [ErrNoProject] when project is empty.
// "serena__query_project" runs a read-only tool against another project Serena
// knows; its symbolic tools need Serena's project server running, its file and
// search tools do not.
//
// [ToolPack.Instructions] returns fixed guidance written for this pack, naming
// the project and stating that Serena counts lines from 0, and, unless readOnly,
// how to edit.
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

	return &codingTools{mcp: client, project: project, readOnly: readOnly}, nil
}

func (c *codingTools) Instructions(_ context.Context) *mcp.Instruction {
	text := fmt.Sprintf(serenaInstruction, c.project)
	if !c.readOnly {
		text += "\n\n" + serenaEditInstruction
	}

	return &mcp.Instruction{
		Name: serenaName,
		Text: text,
	}
}

func (c *codingTools) Close() error {
	return c.mcp.Close()
}

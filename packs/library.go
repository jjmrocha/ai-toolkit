package packs

import (
	"context"

	"github.com/jjmrocha/ai-toolkit/mcp"
	"github.com/jjmrocha/ai-toolkit/tools"
)

const context7Version = "4.4.4"

// Context7MCPConfig returns the [mcp.ClientConfig] [LibraryTools] starts the
// context7 server with. npx runs a fixed release of the package, not the latest.
// The server gets CONTEXT7_API_KEY when the calling process sets it, and runs
// without a key, at lower rate limits, when it does not. Each call returns a new
// value, so it can be changed (to use another version, say) and passed to
// [mcp.NewClient] directly.
func Context7MCPConfig() mcp.ClientConfig {
	return mcp.ClientConfig{
		Name:       "context7",
		Command:    npxCommand,
		Args:       []string{"-y", "@upstash/context7-mcp@" + context7Version},
		InheritEnv: []string{"CONTEXT7_API_KEY"},
	}
}

// LibraryConfig holds the choices [LibraryTools] starts context7 with.
type LibraryConfig struct {
	// NoToolPrefix registers the tools under their own names, such as
	// "query-docs", instead of "context7__query-docs". It sets
	// [mcp.ClientConfig.NoToolPrefix].
	NoToolPrefix bool
}

type libraryTools struct {
	mcp *mcp.Client
}

// LibraryTools registers the tools of context7 (https://github.com/upstash/context7)
// in m: current documentation and code examples for libraries and frameworks.
// It needs the npx executable on PATH and Node.js 20.18.1 or newer. An API key
// is optional; see [Context7MCPConfig]. The tools are
// "context7__resolve-library-id", which finds a library's context7 id, and
// "context7__query-docs", which answers a question from that library's
// documentation. When [LibraryConfig.NoToolPrefix] is set, they are
// "resolve-library-id" and "query-docs".
//
// If registration fails, the server is stopped before LibraryTools returns.
// With [LibraryConfig.NoToolPrefix], that includes [mcp.ErrToolNameTaken] when
// one of the tool names is already in m. If the server dies later, its tools are
// removed from m.
func LibraryTools(ctx context.Context, m *tools.ToolBox, cfg LibraryConfig) (ToolPack, error) {
	mcpConfig := Context7MCPConfig()
	mcpConfig.NoToolPrefix = cfg.NoToolPrefix

	client, err := mcp.NewClient(ctx, mcpConfig, m)
	if err != nil {
		return nil, err
	}

	return &libraryTools{mcp: client}, nil
}

func (l *libraryTools) Instructions(_ context.Context) *mcp.Instruction {
	return l.mcp.Instructions()
}

func (l *libraryTools) Close() error {
	l.mcp.Close()

	return nil
}

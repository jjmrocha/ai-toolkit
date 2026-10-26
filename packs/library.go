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

type libraryTools struct {
	mcp *mcp.Client
}

// LibraryTools registers the tools of context7 (https://github.com/upstash/context7)
// in m: current documentation and code examples for libraries and frameworks.
// It needs the npx executable on PATH and Node.js 20.18.1 or newer. An API key
// is optional; see [Context7MCPConfig]. The tools are
// "context7__resolve-library-id", which finds a library's context7 id, and
// "context7__query-docs", which answers a question from that library's
// documentation.
//
// If registration fails, the server is stopped before LibraryTools returns. If
// the server dies later, its tools are removed from m.
func LibraryTools(ctx context.Context, m *tools.ToolBox) (ToolPack, error) {
	client, err := mcp.NewClient(ctx, Context7MCPConfig(), m)
	if err != nil {
		return nil, err
	}

	return &libraryTools{mcp: client}, nil
}

func (l *libraryTools) Instructions(_ context.Context) *mcp.Instruction {
	return l.mcp.Instructions()
}

func (l *libraryTools) Close() error {
	return l.mcp.Close()
}

package packs

import (
	"context"

	"github.com/jjmrocha/ai-toolkit/mcp"
	"github.com/jjmrocha/ai-toolkit/tools"
)

// SequentialThinkingMCPConfig returns the [mcp.ClientConfig] [ThinkingTools]
// starts the sequential-thinking server with. Each call returns a new value, so
// it can be changed (to pin a version, say) and passed to [mcp.NewClient]
// directly.
func SequentialThinkingMCPConfig() mcp.ClientConfig {
	return mcp.ClientConfig{
		Name:    "thinking",
		Command: "npx",
		Args:    []string{"-y", "@modelcontextprotocol/server-sequential-thinking"},
	}
}

type thinkingTools struct {
	mcp *mcp.Client
}

// ThinkingTools registers the sequential-thinking tool in m, served by the
// reference MCP server
// (https://github.com/modelcontextprotocol/servers/tree/main/src/sequentialthinking).
// It needs the npx executable on PATH and no API key. The tool is
// "thinking__sequentialthinking": the model works a problem as numbered
// thoughts it can revise or branch from.
//
// The server keeps the thought history in memory, so it lasts as long as the
// pack.
//
// If registration fails, the server is stopped before ThinkingTools returns. If
// the server dies later, its tool is removed from m.
func ThinkingTools(ctx context.Context, m *tools.ToolBox) (ToolPack, error) {
	client, err := mcp.NewClient(ctx, SequentialThinkingMCPConfig())
	if err != nil {
		return nil, err
	}

	err = client.RegisterTools(ctx, m)
	if err != nil {
		_ = client.Close()
		return nil, err
	}

	return &thinkingTools{mcp: client}, nil
}

func (t *thinkingTools) Instructions(_ context.Context) (*mcp.Instruction, error) {
	return t.mcp.Instructions(), nil
}

func (t *thinkingTools) Close() error {
	return t.mcp.Close()
}

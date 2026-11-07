package packs

import (
	"context"

	"github.com/jjmrocha/ai-toolkit/mcp"
	"github.com/jjmrocha/ai-toolkit/tools"
)

const sequentialThinkingVersion = "2.0.0"

// SequentialThinkingMCPConfig returns the [mcp.ClientConfig] [ThinkingTools]
// starts the sequential-thinking server with. npx runs a fixed release of the
// package, not the latest. Each call returns a new value, so it can be changed
// (to use another version, say) and passed to [mcp.NewClient] directly.
func SequentialThinkingMCPConfig() mcp.ClientConfig {
	return mcp.ClientConfig{
		Name:    "thinking",
		Command: npxCommand,
		Args:    []string{"-y", "@modelcontextprotocol/server-sequential-thinking@" + sequentialThinkingVersion},
	}
}

// ThinkingConfig holds the choices [ThinkingTools] starts the sequential-thinking server with.
type ThinkingConfig struct {
	// NoToolPrefix registers the tools under their own names, such as
	// "sequentialthinking", instead of "thinking__sequentialthinking". It sets
	// [mcp.ClientConfig.NoToolPrefix].
	NoToolPrefix bool
}

type thinkingTools struct {
	mcp *mcp.Client
}

// ThinkingTools registers the sequential-thinking tool in m, served by the
// reference MCP server
// (https://github.com/modelcontextprotocol/servers/tree/main/src/sequentialthinking).
// It needs the npx executable on PATH, Node.js 24 or newer and no API key. The
// tool is "thinking__sequentialthinking", or "sequentialthinking" when
// [ThinkingConfig.NoToolPrefix] is set: the model works a problem as numbered
// thoughts it can revise or branch from.
//
// The server keeps the thought history in memory, so it lasts as long as the
// pack.
//
// If registration fails, the server is stopped before ThinkingTools returns.
// With [ThinkingConfig.NoToolPrefix], that includes [mcp.ErrToolNameTaken] when
// the tool name is already in m. If the server dies later, its tool is removed
// from m.
func ThinkingTools(ctx context.Context, m *tools.ToolBox, cfg ThinkingConfig) (ToolPack, error) {
	mcpConfig := SequentialThinkingMCPConfig()
	mcpConfig.NoToolPrefix = cfg.NoToolPrefix

	client, err := mcp.NewClient(ctx, mcpConfig, m)
	if err != nil {
		return nil, err
	}

	return &thinkingTools{mcp: client}, nil
}

func (t *thinkingTools) Instructions(_ context.Context) *mcp.Instruction {
	return t.mcp.Instructions()
}

func (t *thinkingTools) Close() error {
	t.mcp.Close()

	return nil
}

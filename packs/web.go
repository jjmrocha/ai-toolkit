package packs

import (
	"context"
	"time"

	"github.com/jjmrocha/ai-toolkit/mcp"
	"github.com/jjmrocha/ai-toolkit/tools"
)

// DonSeTchMCPConfig returns the [mcp.ClientConfig] that [WebTools] starts
// DonSeTch from. Every call returns a fresh value that shares nothing with the
// pack, so the returned config can be adjusted — pinned to a revision, say —
// and passed to [mcp.NewClient] directly.
func DonSeTchMCPConfig() mcp.ClientConfig {
	return mcp.ClientConfig{
		Name:            "donsetch",
		Command:         "donsetch",
		Args:            []string{"mcp", "--supervised"},
		ToolCallTimeout: 15 * time.Minute,
	}
}

// WebTools registers web search, page fetching and site crawling in m, served
// by DonSeTch (https://github.com/dondai44423/donsetch). It needs the donsetch
// executable on PATH and no API key. The tools are registered as
// "donsetch__web_search", "donsetch__web_fetch" and "donsetch__web_crawl", and
// the returned [ToolPack] removes them again.
//
// A registration that fails stops the server before returning, leaving nothing
// behind. A server that later dies on its own also removes its own tools from
// m, so a pack whose server is gone leaves no tool the model can still call.
func WebTools(ctx context.Context, m *tools.ToolBox) (ToolPack, error) {
	client, err := mcp.NewClient(ctx, DonSeTchMCPConfig())
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

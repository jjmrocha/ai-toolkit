package packs

import (
	"context"
	"time"

	"github.com/jjmrocha/ai-toolkit/mcp"
	"github.com/jjmrocha/ai-toolkit/tools"
)

// DonSeTchMCPConfig returns the [mcp.ClientConfig] [WebTools] starts DonSeTch
// with. Each call returns a new value, so it can be changed (to pin a revision,
// say) and passed to [mcp.NewClient] directly.
func DonSeTchMCPConfig() mcp.ClientConfig {
	return mcp.ClientConfig{
		Name:            "donsetch",
		Command:         "donsetch",
		Args:            []string{"mcp", "--supervised"},
		ToolCallTimeout: 15 * time.Minute,
	}
}

type webTools struct {
	mcp *mcp.Client
}

// WebTools registers web search, page fetching and site crawling in m, served
// by DonSeTch (https://github.com/dondai44423/donsetch). It needs the donsetch
// executable on PATH and no API key. The tools are "donsetch__web_search",
// "donsetch__web_fetch" and "donsetch__web_crawl".
//
// If registration fails, the server is stopped before WebTools returns. If the
// server dies later, its tools are removed from m.
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

	return &webTools{mcp: client}, nil
}

func (w *webTools) Instructions(_ context.Context) (*mcp.Instruction, error) {
	return w.mcp.Instructions(), nil
}

func (w *webTools) Close() error {
	return w.mcp.Close()
}

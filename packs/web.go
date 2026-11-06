package packs

import (
	"context"
	"time"

	"github.com/jjmrocha/ai-toolkit/mcp"
	"github.com/jjmrocha/ai-toolkit/tools"
)

const donsetchVersion = "4.10.2"

// DonSeTchMCPConfig returns the [mcp.ClientConfig] [WebTools] starts DonSeTch
// with. npx runs a fixed release of the package, not the latest. Its
// "web_screenshot" tool is excluded, because an image result reaches the model
// only as a placeholder. Each call returns a new value, so it can be changed
// (to use another version, say) and passed to [mcp.NewClient] directly.
func DonSeTchMCPConfig() mcp.ClientConfig {
	return mcp.ClientConfig{
		Name:            "donsetch",
		Command:         npxCommand,
		Args:            []string{"-y", "donsetch@" + donsetchVersion, "mcp", "--supervised"},
		ExcludedTools:   []string{"web_screenshot"},
		ToolCallTimeout: 15 * time.Minute,
	}
}

type webTools struct {
	mcp *mcp.Client
}

// WebTools registers web search, page fetching and site crawling in m, served
// by DonSeTch (https://github.com/dondai44423/donsetch). It needs the npx
// executable on PATH and no API key. On its first run, the package downloads
// DonSeTch's prebuilt binary from GitHub. The tools are "donsetch__web_search",
// "donsetch__web_fetch" and "donsetch__web_crawl", or "web_search", "web_fetch"
// and "web_crawl" when noToolPrefix is true.
//
// If registration fails, the server is stopped before WebTools returns. With
// noToolPrefix, that includes [mcp.ErrToolNameTaken] when one of the tool names
// is already in m. If the server dies later, its tools are removed from m.
func WebTools(ctx context.Context, m *tools.ToolBox, noToolPrefix bool) (ToolPack, error) {
	cfg := DonSeTchMCPConfig()
	cfg.NoToolPrefix = noToolPrefix

	client, err := mcp.NewClient(ctx, cfg, m)
	if err != nil {
		return nil, err
	}

	return &webTools{mcp: client}, nil
}

func (w *webTools) Instructions(_ context.Context) *mcp.Instruction {
	return w.mcp.Instructions()
}

func (w *webTools) Close() error {
	return w.mcp.Close()
}

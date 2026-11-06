package packs

import (
	"context"
	"strings"

	"github.com/jjmrocha/ai-toolkit/mcp"
	"github.com/jjmrocha/ai-toolkit/tools"
)

const (
	yfmcpVersion = "0.14.0"
	yfinanceName = "yfinance"
)

const yfinanceInstruction = `The yfinance tools read market data from Yahoo
Finance. Quotes may be delayed, so give the time of a price when it matters.
Leave chart_type unset when you call yfinance__yfinance_get_price_history. A
chart reaches you only as a placeholder, not as an image you can read, so ask
for the price table and read the numbers instead. When a tool reports a rate
limit, wait before calling it again.`

// YFinanceMCPConfig returns the [mcp.ClientConfig] [FinanceTools] starts the
// yfmcp server with. uvx runs a fixed release of the package, not the latest.
// Each call returns a new value, so it can be changed (to use another version,
// say) and passed to [mcp.NewClient] directly.
func YFinanceMCPConfig() mcp.ClientConfig {
	return mcp.ClientConfig{
		Name:    yfinanceName,
		Command: "uvx",
		Args:    []string{"--from", "yfmcp==" + yfmcpVersion, "yfmcp"},
	}
}

type financeTools struct {
	mcp          *mcp.Client
	noToolPrefix bool
}

// FinanceTools registers the tools of yfmcp (https://github.com/narumiruna/yfinance-mcp)
// in m: market data from Yahoo Finance, including quotes, price history,
// financial statements, analyst estimates, news, holders, funds, options and
// screeners. The tools are named "yfinance__yfinance_<name>", or
// "yfinance_<name>" when noToolPrefix is true. It needs the uvx executable on
// PATH and no API key. On its first run, uvx downloads Python 3.12 or newer and
// the package's dependencies. Every symbol and search the model asks about is
// sent to Yahoo.
//
// [ToolPack.Instructions] returns fixed guidance written for this pack, naming
// the tools as they are registered.
//
// If registration fails, the server is stopped before FinanceTools returns.
// With noToolPrefix, that includes [mcp.ErrToolNameTaken] when one of the tool
// names is already in m. If the server dies later, its tools are removed from m.
func FinanceTools(ctx context.Context, m *tools.ToolBox, noToolPrefix bool) (ToolPack, error) {
	cfg := YFinanceMCPConfig()
	cfg.NoToolPrefix = noToolPrefix

	client, err := mcp.NewClient(ctx, cfg, m)
	if err != nil {
		return nil, err
	}

	return &financeTools{mcp: client, noToolPrefix: noToolPrefix}, nil
}

func (f *financeTools) Instructions(_ context.Context) *mcp.Instruction {
	text := yfinanceInstruction
	if f.noToolPrefix {
		text = strings.ReplaceAll(text, yfinanceName+"__", "")
	}

	return &mcp.Instruction{
		Name: yfinanceName,
		Text: text,
	}
}

func (f *financeTools) Close() error {
	return f.mcp.Close()
}

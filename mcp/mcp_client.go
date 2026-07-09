// Package mcp connects a stdio-based MCP (Model Context Protocol) server to a
// tools.ToolBox. A Client launches the server as a child process, discovers the
// tools it offers, and registers each one in the ToolBox so a model can call
// them like any other tool.
//
// A Client drives exactly one MCP server over its stdin/stdout. Tool calls are
// serialized by the transport, so at most one request is in flight at a time.
// Context cancellation is best-effort: a request blocked waiting on a silent
// server is not interrupted by its deadline — call Close to abort it.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/tools"
)

const callToolTimeout = 30 * time.Second

// Client registers the tools exposed by a single MCP server into a ToolBox and
// owns the lifetime of that server's process. Create one with NewClient and
// always pair it with a deferred Close.
type Client struct {
	config    ClientConfig
	toolBox   *tools.ToolBox
	transport *stdio
	tools     []string
}

// NewClient launches the MCP server described by cfg and completes the protocol
// handshake. ctx bounds the startup handshake only. It returns ErrNameRequired
// or ErrCommandRequired if cfg is incomplete, or an error if the server fails to
// start, the handshake fails, or the server speaks an unsupported protocol
// version. The server runs until Close is called. Call RegisterTools to bind the
// client to a ToolBox.
func NewClient(ctx context.Context, cfg ClientConfig) (*Client, error) {
	if cfg.Name == "" {
		return nil, ErrNameRequired
	}

	if cfg.Command == "" {
		return nil, ErrCommandRequired
	}

	t, err := newStdIO(ctx, cfg.Command, cfg.Args)
	if err != nil {
		return nil, err
	}

	return &Client{
		config:    cfg,
		transport: t,
	}, nil
}

// Connected reports whether the server's child process is still running. It
// returns false once the process has exited, whether it was closed or died on
// its own.
func (c *Client) Connected() bool {
	return c.transport != nil && c.transport.connected()
}

// Close removes this client's tools from the ToolBox and shuts the server
// process down. Because it does not wait on an in-flight Request, Close also
// aborts a tool call that is stuck waiting on the server. It is safe to call
// more than once.
func (c *Client) Close() error {
	if c.transport == nil {
		return nil
	}

	if c.toolBox != nil {
		for _, tool := range c.tools {
			c.toolBox.RemoveTool(tool)
		}
	}

	return c.transport.close()
}

// RegisterTools queries the server for its tools and registers each one in tb,
// namespaced as "<ClientConfig.Name>__<tool>" and backed by a handler that
// forwards the call to the server. ctx bounds the tools/list request. Tools
// registered here are removed again by Close. It may be called only once per
// client and returns ErrAlreadyRegistered on a later call.
func (c *Client) RegisterTools(ctx context.Context, tb *tools.ToolBox) error {
	if c.toolBox != nil {
		return ErrAlreadyRegistered
	}

	result, err := c.transport.Request(ctx, "tools/list", nil)
	if err != nil {
		return err
	}

	c.toolBox = tb

	list, _ := result["tools"].([]any)
	for _, item := range list {
		spec, ok := item.(map[string]any)
		if !ok {
			continue
		}

		name, _ := spec["name"].(string)
		if name == "" {
			continue
		}

		description, _ := spec["description"].(string)
		schema, _ := spec["inputSchema"].(map[string]any)

		tool := llm.Tool{
			Name:        fmt.Sprintf("%s__%s", c.config.Name, name),
			Description: description,
			Schema:      schema,
		}

		if err := c.toolBox.AddTool(tool, c.makeHandler(name)); err != nil {
			return err
		}
		c.tools = append(c.tools, tool.Name)
	}

	return nil
}

func (c *Client) makeHandler(name string) tools.Handler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		ctx, cancel := context.WithTimeout(ctx, callToolTimeout)
		defer cancel()

		result, err := c.transport.Request(ctx, "tools/call", map[string]any{
			"name":      name,
			"arguments": args,
		})

		if err != nil {
			return "", err
		}

		text, failed := parseToolResult(result)
		if failed {
			return "", fmt.Errorf("tool %s reported an error: %s", name, text)
		}

		return text, nil
	}
}

func parseToolResult(result map[string]any) (text string, failed bool) {
	failed, _ = result["isError"].(bool)
	content, _ := result["content"].([]any)

	parts := make([]string, 0, len(content))
	for _, item := range content {
		part, ok := item.(map[string]any)
		if !ok {
			continue
		}

		if part["type"] != "text" {
			continue
		}

		if t, ok := part["text"].(string); ok {
			parts = append(parts, t)
		}
	}

	if len(parts) > 0 {
		return strings.Join(parts, "\n"), failed
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		return "", failed
	}

	return string(encoded), failed
}

package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/tools"
)

const (
	callToolTimeout    = 120 * time.Second
	listToolsTimeout   = 30 * time.Second
	maxToolPages       = 100
	toolNameHashLength = 6
)

// Client registers the tools exposed by a single MCP server into a
// [tools.ToolBox] and owns the lifetime of that server's process. Create one
// with [NewClient] and always pair it with a deferred [Client.Close].
type Client struct {
	config  ClientConfig
	session *session

	mu      sync.Mutex
	toolBox *tools.ToolBox
	tools   []string
}

// NewClient launches the MCP server described by cfg and completes the protocol
// handshake. ctx bounds the startup handshake only. It returns
// [ErrNameRequired] or [ErrCommandRequired] if cfg is incomplete, or an error if
// the server fails to start, the handshake fails, or the server speaks an
// unsupported protocol version. The server runs until [Client.Close] is called.
// Call [Client.RegisterTools] to bind the client to a [tools.ToolBox].
func NewClient(ctx context.Context, cfg ClientConfig) (*Client, error) {
	if cfg.Name == "" {
		return nil, ErrNameRequired
	}

	if cfg.Command == "" {
		return nil, ErrCommandRequired
	}

	c := &Client{config: cfg}

	s, err := newSession(ctx, cfg.Command, cfg.Args, c.unregisterTools)
	if err != nil {
		return nil, err
	}

	c.session = s

	return c, nil
}

// Connected reports whether the server's child process is still running. It
// returns false once the process has exited, whether it was closed, died on its
// own, or was stopped because its output could no longer be read.
func (c *Client) Connected() bool {
	return c.session.connected()
}

// Close shuts the server process down and removes this client's tools from the
// [tools.ToolBox]. A call or a registration still waiting on the server is
// aborted rather than waited out. It is safe to call more than once.
func (c *Client) Close() error {
	c.session.close()

	c.unregisterTools()

	return nil
}

func (c *Client) unregisterTools() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, tool := range c.tools {
		c.toolBox.Remove(tool)
	}

	c.tools = nil
}

// RegisterTools queries the server for its tools and registers each one in tb,
// namespaced as "<ClientConfig.Name>__<tool>" and backed by a handler that
// forwards the call to the server. A namespaced name the providers would reject
// is rewritten rather than dropped; the server is still called by the name it
// published. ctx bounds every page of the tools/list request. Tools registered
// here are removed again by [Client.Close]. Only a successful registration
// latches: it returns [ErrAlreadyRegistered] on a later call, while a failed one
// leaves the client free to try again.
func (c *Client) RegisterTools(ctx context.Context, tb *tools.ToolBox) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.toolBox != nil {
		return ErrAlreadyRegistered
	}

	specs, err := c.listTools(ctx)
	if err != nil {
		return err
	}

	registered := make([]string, 0, len(specs))

	for _, spec := range specs {
		tool := llm.Tool{
			Name:        c.toolName(spec.name, registered),
			Description: spec.description,
			Schema:      spec.schema,
		}

		if err := tb.Add(tool, c.makeHandler(spec.name)); err != nil {
			for _, name := range registered {
				tb.Remove(name)
			}

			return err
		}

		registered = append(registered, tool.Name)
	}

	c.toolBox = tb
	c.tools = registered

	return nil
}

func (c *Client) toolName(tool string, taken []string) string {
	original := c.config.Name + "__" + tool
	name := tools.SanitizeToolName(original)

	if len(name) <= tools.MaxToolNameLength && !slices.Contains(taken, name) {
		return name
	}

	suffix := "_" + hashToolName(original)
	keep := min(len(name), tools.MaxToolNameLength-len(suffix))

	return name[:keep] + suffix
}

func hashToolName(original string) string {
	sum := sha256.Sum256([]byte(original))

	return hex.EncodeToString(sum[:])[:toolNameHashLength]
}

func (c *Client) listTools(ctx context.Context) ([]toolSpec, error) {
	ctx, cancel := context.WithTimeout(ctx, listToolsTimeout)
	defer cancel()

	var specs []toolSpec
	var params map[string]any

	for range maxToolPages {
		result, err := c.session.Request(ctx, "tools/list", params)
		if err != nil {
			return nil, err
		}

		specs = append(specs, parseToolSpecs(result)...)

		cursor, _ := result["nextCursor"].(string)
		if cursor == "" {
			return specs, nil
		}

		params = map[string]any{"cursor": cursor}
	}

	return nil, fmt.Errorf("%w: stopped after %d pages", ErrTooManyToolPages, maxToolPages)
}

func parseToolSpecs(result map[string]any) []toolSpec {
	tools, _ := result["tools"].([]any)

	specs := make([]toolSpec, 0, len(tools))
	for _, tool := range tools {
		toolDef, ok := tool.(map[string]any)
		if !ok {
			continue
		}

		name, _ := toolDef["name"].(string)
		if name == "" {
			continue
		}

		description, _ := toolDef["description"].(string)
		schema, _ := toolDef["inputSchema"].(map[string]any)

		spec := toolSpec{name: name, description: description, schema: schema}
		specs = append(specs, spec)
	}

	return specs
}

func (c *Client) makeHandler(name string) tools.Handler {
	return func(ctx context.Context, args map[string]any) (string, error) {
		ctx, cancel := context.WithTimeout(ctx, callToolTimeout)
		defer cancel()

		result, err := c.session.Request(ctx, "tools/call", map[string]any{
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

func parseToolResult(result map[string]any) (string, bool) {
	failed, _ := result["isError"].(bool)
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

package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"sync"
	"time"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/tools"
	"github.com/jjmrocha/go-algo/token"
)

const (
	defaultToolCallTimeout = 60 * time.Second
	listToolsTimeout       = 30 * time.Second
	toolNameHashLength     = 6
)

// Client registers one MCP server's tools in a [tools.ToolBox] and owns the
// server's process. Create one with [NewClient] and always defer
// [Client.Close]. After [Client.RegisterTools], a server that changes its tool
// list gets its tools registered again automatically. It is safe for
// concurrent use.
type Client struct {
	config ClientConfig

	session  *sdkClient
	requests *pendingRequest

	mu        sync.Mutex
	connected bool
	toolBox   *tools.ToolBox
	tools     []string
}

// NewClient launches the MCP server described by cfg and completes the
// handshake, which ctx bounds. The server runs until [Client.Close]. It returns
// [ErrNameRequired] or [ErrCommandRequired] if cfg is incomplete, or an error
// if the server fails to start or the handshake fails. Call
// [Client.RegisterTools] to register its tools.
func NewClient(ctx context.Context, cfg ClientConfig) (*Client, error) {
	if cfg.Name == "" {
		return nil, ErrNameRequired
	}

	if cfg.Command == "" {
		return nil, ErrCommandRequired
	}

	c := &Client{
		config:    cfg,
		connected: true,
		requests:  newPendingRequest(),
	}

	cb := callBacks{
		onProgress:    c.onProgress,
		onToolsChange: c.onToolsChange,
		onDisconnect:  c.onDisconnect,
	}

	s, err := newSDKClient(ctx, cfg, cb)
	if err != nil {
		return nil, err
	}

	c.session = s

	return c, nil
}

// Name returns the client's name, which prefixes the tools it registers.
func (c *Client) Name() string {
	return c.config.Name
}

func (c *Client) onDisconnect(_ error) {
	c.disconnected()
}

func (c *Client) disconnected() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.removeTools()
	c.connected = false
}

func (c *Client) onToolsChange() {
	c.mu.Lock()
	tb := c.toolBox
	c.mu.Unlock()

	if tb == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), listToolsTimeout)
	defer cancel()

	_ = c.RegisterTools(ctx, tb)
}

func (c *Client) onProgress(token string) {
	c.requests.reset(token)
}

// Connected reports whether the server process is still running. It is false
// once the process has exited, whether it was closed, died, or was stopped
// because its output could not be read.
func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.connected
}

// Instructions returns the instructions the server sent in its handshake,
// labeled with the client's name, or nil if it sent none. They are captured at
// startup and do not change.
func (c *Client) Instructions() *Instruction {
	instructions := c.session.instructions()
	if instructions == "" {
		return nil
	}

	return &Instruction{
		Name: c.config.Name,
		Text: instructions,
	}
}

// Close stops the server process and removes the client's tools from the
// [tools.ToolBox]. Calls still waiting on the server are aborted. It is safe to
// call more than once.
func (c *Client) Close() error {
	c.session.close()

	c.disconnected()

	return nil
}

// RegisterTools lists the server's tools and registers each one in tb as
// "<ClientConfig.Name>__<tool>", with a handler that forwards the call to the
// server. A namespaced name the providers would reject is rewritten, not
// dropped; the server is still called by its own name. Tools in
// [ClientConfig.ExcludedTools] are skipped. ctx bounds the tools/list request.
// [Client.Close] removes the tools again.
//
// Calling it again replaces the tools the previous call registered. That is how
// the client refreshes when the server changes its tool list.
//
// A server whose handshake declared no tools capability is not asked for a
// list: nothing is registered, the call succeeds, and the server keeps running
// for whatever else it offers. A server that declared no capabilities at all is
// asked anyway.
func (c *Client) RegisterTools(ctx context.Context, tb *tools.ToolBox) error {
	if !c.session.supportsTools() {
		return nil
	}

	specs, err := c.session.listTools(ctx)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.removeTools()

	registered := make([]string, 0, len(specs))

	for _, spec := range specs {
		if c.excluded(spec.name) {
			continue
		}

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

// CallTool calls the server's tool named tool with args and returns its text
// result, as a tool registered by [Client.RegisterTools] would: under
// [ClientConfig.ToolCallTimeout], with nil args sent as an empty object. tool
// is the server's own name for it, not the namespaced one, and it is called
// even if [ClientConfig.ExcludedTools] lists it. It returns an error when the
// call fails or the server reports one.
func (c *Client) CallTool(ctx context.Context, tool string, args map[string]any) (string, error) {
	return c.makeHandler(tool)(ctx, args)
}

func (c *Client) removeTools() {
	for _, tool := range c.tools {
		c.toolBox.Remove(tool)
	}

	c.tools = nil
}

func (c *Client) excluded(tool string) bool {
	return slices.Contains(c.config.ExcludedTools, tool)
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

func (c *Client) makeHandler(name string) tools.Handler {
	return func(parent context.Context, args map[string]any) (string, error) {
		toolTimeout := defaultToolCallTimeout

		if c.config.ToolCallTimeout > 0 {
			toolTimeout = c.config.ToolCallTimeout
		}

		requestToken := token.New()
		ctx := c.requests.newResettableTimeout(parent, requestToken, toolTimeout)
		defer func() {
			c.requests.stop(requestToken)
		}()

		if args == nil {
			args = map[string]any{}
		}

		result, err := c.session.execute(ctx, requestToken, name, args)
		if err != nil {
			return "", err
		}

		return result, nil
	}
}

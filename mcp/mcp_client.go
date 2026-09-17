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

// Client registers the tools exposed by a single MCP server into a
// [tools.ToolBox] and owns the lifetime of that server's process. Create one
// with [NewClient] and always pair it with a deferred [Client.Close]. Once
// [Client.RegisterTools] has bound the client to a ToolBox, a server that
// announces a change to its tool list has those tools registered again
// automatically. It is safe for concurrent use.
type Client struct {
	config ClientConfig

	session  *sdkClient
	requests *pendingRequest

	mu        sync.Mutex
	connected bool
	toolBox   *tools.ToolBox
	tools     []string
}

// NewClient launches the MCP server described by cfg and completes the protocol
// handshake. ctx bounds the startup handshake only. It returns
// [ErrNameRequired] or [ErrCommandRequired] if cfg is incomplete, or an error if
// the server fails to start or the handshake fails. The server runs until
// [Client.Close] is called. Call [Client.RegisterTools] to bind the client to a
// [tools.ToolBox].
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

// Connected reports whether the server's child process is still running. It
// returns false once the process has exited, whether it was closed, died on its
// own, or was stopped because its output could no longer be read.
func (c *Client) Connected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.connected
}

// Close shuts the server process down and removes this client's tools from the
// [tools.ToolBox]. A call still waiting on the server is aborted rather than
// waited out. It is safe to call more than once.
func (c *Client) Close() error {
	c.session.close()

	c.disconnected()

	return nil
}

// RegisterTools queries the server for its tools and registers each one in tb,
// namespaced as "<ClientConfig.Name>__<tool>" and backed by a handler that
// forwards the call to the server. A namespaced name the providers would reject
// is rewritten rather than dropped; the server is still called by the name it
// published. ctx bounds the tools/list request. Tools registered here are
// removed again by [Client.Close].
//
// Calling it again replaces the tools the previous call registered, which is how
// the client refreshes itself when the server announces a change to its tool
// list.
//
// A server whose handshake declared no tools capability is never asked for a
// tool list: nothing is registered and the call succeeds, leaving the server
// running for whatever else it offers. A server that declares no capabilities at
// all is asked anyway.
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

func (c *Client) removeTools() {
	for _, tool := range c.tools {
		c.toolBox.Remove(tool)
	}

	c.tools = nil
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

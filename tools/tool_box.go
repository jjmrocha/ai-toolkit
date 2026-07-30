// Package tools helps wire model tool calls to Go code. It provides a ToolBox
// that pairs each llm.Tool definition with the function that runs it, an
// ObjectBuilder for constructing the JSON Schema that describes a tool's
// parameters without hand-writing nested maps, and an Arguments wrapper for
// reading a call's decoded arguments back out with typed accessors.
package tools

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sync"

	"github.com/jjmrocha/ai-toolkit/llm"
)

// Handler executes a tool call. It receives the caller's context — honor it for
// cancellation and deadlines in any I/O — and the decoded arguments from the
// model (values arrive with JSON types, so numbers are float64), and returns the
// result string sent back to the model, or an error.
type Handler func(context.Context, map[string]any) (string, error)

type toolFn struct {
	tool    llm.Tool
	handler Handler
}

// ToolBox is a registry that pairs llm.Tool definitions with the functions that
// execute them, bridging a tool call requested by the model and your code:
// register tools with Add, expose their definitions to the model with
// Tools, and run a requested call with Execute.
//
// A ToolBox is safe for concurrent use: tools may be added and removed while
// other goroutines list or execute them, as happens when an MCP server
// registers or drops its tools at runtime.
type ToolBox struct {
	mu    sync.RWMutex
	tools map[string]toolFn
}

// NewToolBox returns an empty ToolBox ready for tool registration.
func NewToolBox() *ToolBox {
	return &ToolBox{
		tools: make(map[string]toolFn),
	}
}

// toolNamePattern matches the tool names accepted by the providers: 1 to 64
// characters (Anthropic's limit, the strictest), each a letter, digit,
// underscore, or hyphen.
var toolNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// Add registers tool together with the handler that executes it. The
// handler is keyed by tool.Name; registering a tool whose name already exists
// replaces the previous entry.
//
// It returns ErrInvalidToolName without registering anything if tool.Name is
// empty or contains a character the providers reject (only letters, digits,
// underscore, and hyphen are allowed, up to 64 characters), or ErrNilHandler
// if handler is nil.
func (tb *ToolBox) Add(tool llm.Tool, handler Handler) error {
	if !toolNamePattern.MatchString(tool.Name) {
		return fmt.Errorf("%w: %q", ErrInvalidToolName, tool.Name)
	}

	if handler == nil {
		return fmt.Errorf("%w: %q", ErrNilHandler, tool.Name)
	}

	t := toolFn{
		tool:    tool,
		handler: handler,
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.tools[tool.Name] = t

	return nil
}

// Remove unregisters the tool with the given name. It is a no-op if no such
// tool is registered.
func (tb *ToolBox) Remove(name string) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	delete(tb.tools, name)
}

// Tools returns the definitions of all registered tools, sorted by name,
// suitable for passing to llm.LLM.Chat. The stable order keeps the tool
// section of the prompt identical across requests, which providers with
// prompt caching rely on.
func (tb *ToolBox) Tools() []llm.Tool {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	tools := make([]llm.Tool, 0, len(tb.tools))

	for _, name := range slices.Sorted(maps.Keys(tb.tools)) {
		tools = append(tools, tb.tools[name].tool)
	}

	return tools
}

// Execute runs the handler for the requested tool call and wraps its result
// in an llm.ToolMessage ready to append to the conversation. ctx is passed to
// the handler for cancellation and deadlines. It returns ErrToolNotFound if no
// tool matches call.Name, or a wrapped error if the handler itself fails. The
// returned message correlates by both ToolCallID and ToolName so it works with
// either provider.
func (tb *ToolBox) Execute(ctx context.Context, call llm.ToolCall) (*llm.ToolMessage, error) {
	tb.mu.RLock()
	fn, ok := tb.tools[call.Name]
	tb.mu.RUnlock()

	if !ok {
		return nil, ErrToolNotFound
	}

	handler := fn.handler

	result, err := handler(ctx, call.Arguments)
	if err != nil {
		return nil, fmt.Errorf("error executing tool %s: %w", call.Name, err)
	}

	return &llm.ToolMessage{
		ToolCallID: call.ID,
		ToolName:   call.Name,
		Content:    result,
	}, nil
}

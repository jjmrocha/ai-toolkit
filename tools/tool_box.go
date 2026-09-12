package tools

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/jjmrocha/ai-toolkit/llm"
)

// MaxToolNameLength is the longest tool name the providers accept: Anthropic's
// limit, the strictest of them.
const MaxToolNameLength = 64

// Handler executes a tool call. It returns the result string sent back to the
// model, or an error. The context is the caller's; honor it for cancellation and
// deadlines in any I/O. The arguments are decoded from the model's request, so
// they carry JSON types and numbers arrive as float64. [Arguments] reads them
// back with typed accessors.
type Handler func(context.Context, map[string]any) (string, error)

type toolFn struct {
	tool    llm.Tool
	handler Handler
}

// Interceptor decides whether a tool call may run. It is called by
// [ToolBox.Execute] after the tool is found and before its [Handler] runs, with
// the same context and the call the model requested. Returning nil lets the call
// proceed; returning an error blocks it, and the handler never runs. The
// arguments in call are the ones the handler would run with and must not be
// modified.
type Interceptor func(context.Context, llm.ToolCall) error

// ToolBox is a registry that pairs [llm.Tool] definitions with the functions
// that execute them, bridging a tool call requested by the model and your code:
// register tools with [ToolBox.Add], expose their definitions to the model with
// [ToolBox.Tools], and run a requested call with [ToolBox.Execute].
//
// A ToolBox is safe for concurrent use: tools may be added and removed while
// other goroutines list or execute them, as happens when an MCP server
// registers or drops its tools at runtime.
type ToolBox struct {
	mu          sync.RWMutex
	tools       map[string]toolFn
	interceptor Interceptor
}

// NewToolBox returns an empty [ToolBox] ready for tool registration.
func NewToolBox() *ToolBox {
	return &ToolBox{
		tools: make(map[string]toolFn),
	}
}

// ValidToolName reports whether name is accepted by [ToolBox.Add]: 1 to
// [MaxToolNameLength] characters, each a letter, digit, underscore, or hyphen.
// Callers that derive a tool name from an outside source, such as an MCP
// server, can check it here instead of rediscovering the providers' rules.
func ValidToolName(name string) bool {
	if name == "" || len(name) > MaxToolNameLength {
		return false
	}

	for _, r := range name {
		if !validToolNameRune(r) {
			return false
		}
	}

	return true
}

// SanitizeToolName replaces every character the providers reject with an
// underscore, leaving a pure ASCII name that is safe to truncate by byte. It
// does not bound the length: a caller that must fit [MaxToolNameLength] still
// has to shorten the result.
func SanitizeToolName(name string) string {
	return strings.Map(func(r rune) rune {
		if validToolNameRune(r) {
			return r
		}

		return '_'
	}, name)
}

func validToolNameRune(r rune) bool {
	return r == '_' || r == '-' ||
		(r >= '0' && r <= '9') ||
		(r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z')
}

// Add registers tool together with the handler that executes it. The handler is
// keyed by tool.Name; registering a tool whose name already exists replaces the
// previous entry.
//
// It returns [ErrInvalidToolName] without registering anything if tool.Name is
// rejected by [ValidToolName], or [ErrNilHandler] if handler is nil.
func (tb *ToolBox) Add(tool llm.Tool, handler Handler) error {
	if !ValidToolName(tool.Name) {
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
// suitable for passing to [llm.LLM.Chat]. The stable order keeps the tool
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

// SetInterceptor installs i as the gate [ToolBox.Execute] consults before it
// runs any tool, replacing the one set before. A nil i clears it, leaving every
// registered tool to run unguarded, which is the default.
//
// The interceptor applies to every tool in the box, however it was registered —
// by a pack, by an MCP server at runtime, or by the caller — so it is the one
// place to enforce a policy across all of them. The error i returns is wrapped
// and returned by [ToolBox.Execute]; an agent loop that reports a failed call
// back to the model reports a blocked one the same way, so the model is told the
// call was refused and can choose what to do next.
func (tb *ToolBox) SetInterceptor(i Interceptor) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.interceptor = i
}

// Execute runs the handler for the requested tool call and wraps its result in
// an [llm.ToolMessage] ready to append to the conversation. ctx is passed to the
// handler for cancellation and deadlines. It returns [ErrToolNotFound] if no
// tool matches call.Name, a wrapped error if an [Interceptor] set by
// [ToolBox.SetInterceptor] blocks the call, or a wrapped error if the handler
// itself fails. The returned message correlates by both ToolCallID and ToolName
// so it works with either provider.
func (tb *ToolBox) Execute(ctx context.Context, call llm.ToolCall) (*llm.ToolMessage, error) {
	tb.mu.RLock()
	fn, ok := tb.tools[call.Name]
	interceptor := tb.interceptor
	tb.mu.RUnlock()

	if !ok {
		return nil, ErrToolNotFound
	}

	if interceptor != nil {
		if err := interceptor(ctx, call); err != nil {
			return nil, fmt.Errorf("tool call %s blocked: %w", call.Name, err)
		}
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

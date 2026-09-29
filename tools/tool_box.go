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

// MaxToolNameLength is the longest tool name every provider accepts. It is
// Anthropic's limit, the strictest.
const MaxToolNameLength = 64

// Handler runs a tool call and returns the result sent back to the model, or an
// error. The context is the caller's; honor it in any I/O. The arguments are
// decoded from JSON, so numbers arrive as float64. [Arguments] reads them with
// typed accessors.
type Handler func(context.Context, map[string]any) (string, error)

type toolFn struct {
	tool    llm.Tool
	handler Handler
}

// Interceptor decides whether a tool call may run. [ToolBox.Execute] calls it
// after finding the tool and before the [Handler] runs, with the same context
// and the call the model made. Returning an error blocks the call. It must not
// modify the call's arguments.
type Interceptor func(context.Context, llm.ToolCall) error

// ToolBox pairs [llm.Tool] definitions with the handlers that run them.
// [ToolBox.Add] registers a tool, [ToolBox.Tools] lists the definitions for the
// model, and [ToolBox.Execute] runs a call the model made.
//
// A ToolBox is safe for concurrent use. Tools can be added and removed while
// other goroutines list or run them, which is what happens when an MCP server
// changes its tools at runtime.
type ToolBox struct {
	mu          sync.RWMutex
	tools       map[string]toolFn
	interceptor Interceptor
}

// NewToolBox returns an empty [ToolBox].
func NewToolBox() *ToolBox {
	return &ToolBox{
		tools: make(map[string]toolFn),
	}
}

// ValidToolName reports whether [ToolBox.Add] accepts name: 1 to
// [MaxToolNameLength] characters, each a letter, digit, underscore or hyphen.
// Use it to check a name that comes from outside, such as an MCP server.
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
// underscore. The result is pure ASCII, so it can be truncated by byte. It does
// not enforce [MaxToolNameLength]; the caller shortens the result if needed.
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

// Add registers tool with the handler that runs it, replacing any tool of the
// same name. It returns [ErrInvalidToolName], registering nothing, if
// [ValidToolName] rejects tool.Name, and [ErrNilHandler] if handler is nil.
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

// Remove unregisters the tool named name. It does nothing if there is none.
func (tb *ToolBox) Remove(name string) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	delete(tb.tools, name)
}

// Tools returns every registered tool's definition, sorted by name, for
// [llm.LLM.Chat]. The fixed order keeps the tools section of the prompt the same
// across requests, which prompt caching relies on.
func (tb *ToolBox) Tools() []llm.Tool {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	tools := make([]llm.Tool, 0, len(tb.tools))

	for _, name := range slices.Sorted(maps.Keys(tb.tools)) {
		tools = append(tools, tb.tools[name].tool)
	}

	return tools
}

// SetInterceptor sets i as the [Interceptor] [ToolBox.Execute] consults before
// it runs any tool, replacing the previous one. A nil i removes it, which is
// the default.
//
// It covers every tool in the box, whether a pack, an MCP server or the caller
// registered it, so it is the one place to enforce a policy. Execute wraps and
// returns the error i returns. An agent loop that reports failed calls to the
// model reports a blocked one the same way, so the model learns the call was
// refused.
func (tb *ToolBox) SetInterceptor(i Interceptor) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.interceptor = i
}

// Execute runs the handler for call and wraps the result in an
// [llm.ToolMessage] ready to append to the conversation. ctx is passed to the
// handler. It returns [ErrToolNotFound] if no tool matches call.Name, and a
// wrapped error if the [Interceptor] blocks the call or the handler fails. The
// message carries both ToolCallID and ToolName, so it works with every provider.
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

// Tool returns the definition of the tool named name, or false if there is
// none. The Schema map is the one the tool was registered with; do not modify
// it.
func (tb *ToolBox) Tool(name string) (llm.Tool, bool) {
	tb.mu.RLock()
	defer tb.mu.RUnlock()

	fn, ok := tb.tools[name]
	if !ok {
		return llm.Tool{}, false
	}

	return fn.tool, true
}

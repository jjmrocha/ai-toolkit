// Package tools helps wire model tool calls to Go code. It provides a ToolBox
// that pairs each llm.Tool definition with the function that runs it, an
// ObjectBuilder for constructing the JSON Schema that describes a tool's
// parameters without hand-writing nested maps, and an Arguments wrapper for
// reading a call's decoded arguments back out with typed accessors.
package tools

import (
	"context"
	"fmt"
	"regexp"

	"github.com/jjmrocha/ai-toolkit/llm"
)

// Handler executes a tool call. It receives the caller's context — honor it for
// cancellation and deadlines in any I/O — and the decoded arguments from the
// model (values arrive with JSON types, so numbers are float64), and returns the
// result string sent back to the model, or an error.
type Handler func(context.Context, map[string]any) (string, error)

type fn struct {
	tool    llm.Tool
	handler Handler
}

// ToolBox is a registry that pairs llm.Tool definitions with the functions that
// execute them, bridging a tool call requested by the model and your code:
// register tools with AddTool, expose their definitions to the model with
// GetTools, and run a requested call with ExecuteTool.
//
// A ToolBox is not safe for concurrent modification. Register every tool during
// setup, then treat it as read-only; concurrent ExecuteTool/GetTools calls are
// then safe.
type ToolBox struct {
	tools map[string]fn
}

// NewToolBox returns an empty ToolBox ready for tool registration.
func NewToolBox() *ToolBox {
	return &ToolBox{
		tools: make(map[string]fn),
	}
}

// toolNamePattern matches the tool names accepted by the providers: 1 to 128
// characters, each a letter, digit, underscore, or hyphen.
var toolNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

// AddTool registers tool together with the handler that executes it. The
// handler is keyed by tool.Name; registering a tool whose name already exists
// replaces the previous entry.
//
// It returns ErrInvalidToolName without registering anything if tool.Name is
// empty or contains a character the providers reject (only letters, digits,
// underscore, and hyphen are allowed, up to 128 characters), or ErrNilHandler
// if handler is nil.
func (tb *ToolBox) AddTool(tool llm.Tool, handler Handler) error {
	if !toolNamePattern.MatchString(tool.Name) {
		return fmt.Errorf("%w: %q", ErrInvalidToolName, tool.Name)
	}

	if handler == nil {
		return fmt.Errorf("%w: %q", ErrNilHandler, tool.Name)
	}

	t := fn{
		tool:    tool,
		handler: handler,
	}
	tb.tools[tool.Name] = t

	return nil
}

// RemoveTool unregisters the tool with the given name. It is a no-op if no such
// tool is registered.
func (tb *ToolBox) RemoveTool(name string) {
	delete(tb.tools, name)
}

// GetTools returns the definitions of all registered tools, suitable for
// passing to llm.LLM.Chat. The order is unspecified.
func (tb *ToolBox) GetTools() []llm.Tool {
	tools := make([]llm.Tool, 0, len(tb.tools))

	for _, t := range tb.tools {
		tools = append(tools, t.tool)
	}

	return tools
}

// ExecuteTool runs the handler for the requested tool call and wraps its result
// in an llm.ToolMessage ready to append to the conversation. ctx is passed to
// the handler for cancellation and deadlines. It returns ErrToolNotFound if no
// tool matches call.Name, or a wrapped error if the handler itself fails. The
// returned message correlates by both ToolCallID and ToolName so it works with
// either provider.
func (tb *ToolBox) ExecuteTool(ctx context.Context, call llm.ToolCall) (*llm.ToolMessage, error) {
	fn, ok := tb.tools[call.Name]
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

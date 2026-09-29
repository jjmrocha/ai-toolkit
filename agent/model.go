package agent

import (
	"time"

	"github.com/jjmrocha/ai-toolkit/llm"
)

// Metadata describes how one [Agent.Process] call was served. The token counts
// are the final reply's, not the whole round's.
type Metadata struct {
	// Iterations is the number of model/tool rounds before the final reply.
	Iterations int
	// PromptTokens is the final reply's input tokens.
	PromptTokens int
	// OutputTokens is the final reply's generated tokens.
	OutputTokens int
	// TotalTokens is the tokens billed for the final reply.
	TotalTokens int
	// ToolCalls is the number of tools run in the round.
	ToolCalls int
	// StopReason is the provider's own reason the final reply stopped, such as
	// "end_turn" or "max_tokens". A reply cut short shows the provider's length
	// limit value.
	StopReason string
	// LLMDuration is the time spent in model calls.
	LLMDuration time.Duration
	// ToolDuration is the time spent running tools.
	ToolDuration time.Duration
}

// Response is what [Agent.Process] returns: the model's final answer and
// [Metadata] on how it was produced.
type Response struct {
	// Content is the final answer's text.
	Content string
	// Metadata is the round's token usage and timing.
	Metadata Metadata
}

// ModelInfo describes the model an [Agent] uses: provider, name, context
// window and reasoning effort.
type ModelInfo struct {
	// Provider is the model's provider.
	Provider llm.Provider
	// ModelName is the model's display name.
	ModelName string
	// ModelContextSize is the model's context window in tokens, or 0 if unknown.
	ModelContextSize int
	// Effort is the reasoning effort applied to each turn.
	Effort llm.Effort
}

package agent

import (
	"time"

	"github.com/jjmrocha/ai-toolkit/llm"
)

// Metadata reports how a single [Agent.Process] call was served. The token
// counts come from the model's final reply, not the whole round.
type Metadata struct {
	// Iterations is the number of model/tool rounds taken before the final reply.
	Iterations int
	// PromptTokens is the input-token count of the final reply.
	PromptTokens int
	// OutputTokens is the generated-token count of the final reply.
	OutputTokens int
	// TotalTokens is the total tokens billed for the final reply.
	TotalTokens int
	// ToolCalls is how many tools were executed across the round.
	ToolCalls int
	// LLMDuration is the wall-clock time spent in model calls.
	LLMDuration time.Duration
	// ToolDuration is the wall-clock time spent executing tools.
	ToolDuration time.Duration
}

// Response is the result of an [Agent.Process] call: the model's final answer
// and the [Metadata] describing how it was produced.
type Response struct {
	// Content is the assistant's final reply text.
	Content string
	// Metadata reports token usage and timing for the round.
	Metadata Metadata
}

// ModelInfo describes the model an [Agent] is currently using: its name,
// context window, and the reasoning effort applied to each turn.
type ModelInfo struct {
	// Provider is the underlying llm.Provider used to serve the model.
	Provider llm.Provider
	// ModelName is the human-readable name of the active model.
	ModelName string
	// ModelContextSize is the model's context window in tokens, or 0 if it
	// could not be determined.
	ModelContextSize int
	// Effort is the reasoning effort the underlying llm client applies.
	Effort llm.Effort
}

package agent

import "time"

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
	// ModelContextSize is the model's context window in tokens, or 0 if it
	// could not be determined.
	ModelContextSize int
	// ModelName is the human-readable name of the model serving the
	// conversation, or "" if it could not be determined.
	ModelName string
}

// Response is the result of an [Agent.Process] call: the model's final answer
// and the [Metadata] describing how it was produced.
type Response struct {
	// Content is the assistant's final reply text.
	Content string
	// Metadata reports token usage and timing for the round.
	Metadata Metadata
}

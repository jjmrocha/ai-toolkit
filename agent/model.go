package agent

import "time"

// Metadata describes how one [Agent.Process] call was served. The token counts
// are the final reply's, not the whole round's.
type Metadata struct {
	// Iterations is the number of model/tool rounds before the final reply.
	Iterations int
	// PromptTokens is the final reply's input tokens.
	PromptTokens int
	// CacheReadTokens is the part of PromptTokens read from the prompt cache. It
	// is zero for providers without prompt caching.
	CacheReadTokens int
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

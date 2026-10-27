package subagent

// Feedback receives the events of every sub-agent run, each tagged with the
// sub-agent's name, so a caller can follow progress while the run blocks.
// Install one with [Manager.SetFeedback]; [NewManager] installs [NopFeedback].
// Methods are called from inside [Manager.Run]. Runs can happen at the same
// time, so the methods must be safe for concurrent use, and they must not block.
type Feedback interface {
	// SubagentStarted fires when [Manager.Run] starts a run, after it has
	// accepted the prompt and found the agent.
	SubagentStarted(agentName string)
	// SubagentFinished fires once after each [Feedback.SubagentStarted]. err is
	// nil on success, or the error [Manager.Run] returns.
	SubagentFinished(agentName string, err error)
	// ToolCalled fires just before the sub-agent runs the named tool.
	ToolCalled(agentName string, toolName string)
	// TokensUsed fires after each response of the sub-agent, the final answer
	// included. totalTokens is that response's total. cacheReadTokens is the part
	// of that response's prompt read from the prompt cache.
	TokensUsed(agentName string, totalTokens, cacheReadTokens int)
}

// NopFeedback is a [Feedback] that ignores every event. [NewManager] installs
// it. Embed it in your own type to handle only the events you care about:
//
//	type runLogger struct{ subagent.NopFeedback }
//
//	func (runLogger) SubagentStarted(name string) { log.Println(name) }
type NopFeedback struct{}

// SubagentStarted does nothing.
func (NopFeedback) SubagentStarted(_ string) {
}

// SubagentFinished does nothing.
func (NopFeedback) SubagentFinished(_ string, _ error) {
}

// ToolCalled does nothing.
func (NopFeedback) ToolCalled(_ string, _ string) {
}

// TokensUsed does nothing.
func (NopFeedback) TokensUsed(_ string, _, _ int) {
}

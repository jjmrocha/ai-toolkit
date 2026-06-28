package agent

import "errors"

// Errors returned by the agent package.
var (
	// ErrNoSession indicates an operation requires an active session, but none
	// has been started (or it was closed).
	ErrNoSession = errors.New("no session started")
	// ErrNoLLM indicates [New] or [NewWithFeedback] was called with a nil LLM.
	ErrNoLLM = errors.New("no LLM provided")
	// ErrMaxIterations indicates [Agent.Process] hit Config.MaxIterations before
	// the model produced a final reply.
	ErrMaxIterations = errors.New("maximum tool iterations reached")
	// ErrInvalidThreshold indicates Config.CompactionThresholdPercent is outside
	// the valid range of 0 to 100.
	ErrInvalidThreshold = errors.New("compaction threshold percent must be between 0 and 100")
)

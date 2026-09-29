package agent

import "errors"

// Errors returned by the agent package.
var (
	// ErrNoSession is returned when a method needs a session and none has been
	// started, or it was closed.
	ErrNoSession = errors.New("no session started")
	// ErrNoLLM is returned by [New] when the LLM is nil.
	ErrNoLLM = errors.New("no LLM provided")
	// ErrMaxIterations is returned by [Agent.Process] when it reaches
	// Config.MaxIterations before the model's final reply.
	ErrMaxIterations = errors.New("maximum tool iterations reached")
	// ErrInvalidThreshold is returned by [New] when
	// Config.CompactionThresholdPercent is outside 0 to 100.
	ErrInvalidThreshold = errors.New("compaction threshold percent must be between 0 and 100")
)

package subagent

import "errors"

// Errors returned by the subagent package.
var (
	// ErrNoName is returned by [Manager.Register] when Config.Name is empty.
	ErrNoName = errors.New("no agent name provided")
	// ErrNoAgent is returned by [Manager.Register] when the agent it is given is
	// nil.
	ErrNoAgent = errors.New("no agent provided")
	// ErrAgentNotFound is returned by [Manager.Run] when no agent is registered
	// under the name it was given.
	ErrAgentNotFound = errors.New("agent not found")
	// ErrNoPrompt is returned by [Manager.Run] when the prompt is empty.
	ErrNoPrompt = errors.New("no prompt provided")
)

package agent

// Feedback receives an [Agent]'s lifecycle events as they happen, letting a
// caller observe progress without changing the conversation. Implementations
// must not block; an event may fire from within [Agent.Process]. [New] installs
// a default implementation that prints each event to standard output.
type Feedback interface {
	// ToolCalled fires just before the agent executes the named tool.
	ToolCalled(toolName string)
	// ContextCompacted fires when the conversation context is compacted to fit
	// the model's window (see Config.CompactionThresholdPercent).
	ContextCompacted()
	// SessionReset fires when [Agent.ResetSession] clears a session.
	SessionReset()
	// SessionStarted fires when [Agent.StartSession] begins a session.
	SessionStarted()
	// SessionClosed fires when [Agent.Close] ends a session.
	SessionClosed()
}

// --------------------------------------------------------------------------------------------

// NewStdoutFeedback returns a [Feedback] implementation that prints each event
// to standard output. It is the default used by [New].
func NewStdoutFeedback() Feedback {
	return newStdoutFeedback()
}

type stdoutFeedback struct{}

func newStdoutFeedback() stdoutFeedback {
	return stdoutFeedback{}
}

func (stdoutFeedback) ToolCalled(toolName string) {
	println("Tool called:", toolName)
}

func (stdoutFeedback) ContextCompacted() {
	println("Context was compacted")
}

func (stdoutFeedback) SessionReset() {
	println("Session reset")
}

func (stdoutFeedback) SessionStarted() {
	println("New session started")
}

func (stdoutFeedback) SessionClosed() {
	println("Session closed")
}

// --------------------------------------------------------------------------------------------

type defaultFeedback struct{}

func (defaultFeedback) ToolCalled(_ string) {
}

func (defaultFeedback) ContextCompacted() {
}

func (defaultFeedback) SessionReset() {
}

func (defaultFeedback) SessionStarted() {
}

func (defaultFeedback) SessionClosed() {
}

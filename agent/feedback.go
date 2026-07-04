package agent

import (
	"fmt"
	"io"
	"os"
)

// Feedback receives an [Agent]'s lifecycle events as they happen, letting a
// caller observe progress without changing the conversation. Implementations
// must not block; an event may fire from within [Agent.Process]. [New] installs
// a silent default that ignores every event; pass [NewStdoutFeedback] to
// [Agent.SetFeedback] to print them instead.
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

type writerFeedback struct {
	stdout io.Writer
}

// NewStdoutFeedback returns a [Feedback] implementation that prints each event
// to standard output. [New]'s default is silent; install this with
// [Agent.SetFeedback] to opt into printing.
func NewStdoutFeedback() Feedback {
	return NewWriterFeedback(os.Stdout)
}

func NewWriterFeedback(w io.Writer) Feedback {
	return &writerFeedback{
		stdout: w,
	}
}

func (s *writerFeedback) ToolCalled(toolName string) {
	_, _ = fmt.Fprintln(s.stdout, "Tool called:", toolName)
}

func (s *writerFeedback) ContextCompacted() {
	_, _ = fmt.Fprintln(s.stdout, "Context was compacted")
}

func (s *writerFeedback) SessionReset() {
	_, _ = fmt.Fprintln(s.stdout, "Session reset")
}

func (s *writerFeedback) SessionStarted() {
	_, _ = fmt.Fprintln(s.stdout, "New session started")
}

func (s *writerFeedback) SessionClosed() {
	_, _ = fmt.Fprintln(s.stdout, "Session closed")
}

type nullFeedback struct{}

func (nullFeedback) ToolCalled(_ string) {
}

func (nullFeedback) ContextCompacted() {
}

func (nullFeedback) SessionReset() {
}

func (nullFeedback) SessionStarted() {
}

func (nullFeedback) SessionClosed() {
}

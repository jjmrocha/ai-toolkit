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
	// ToolCalled fires just before the agent executes the named tool, with the
	// arguments the model supplied for the call. The arguments are nil when the
	// call carries none, and they arrive with JSON types, so numbers are
	// float64. The map is the one the tool is about to run with and must not be
	// modified.
	ToolCalled(toolName string, args map[string]any)
	// ContextCompacted fires when the conversation context is compacted to fit
	// the model's window (see Config.CompactionThresholdPercent).
	ContextCompacted()
	// ContextCompactionFailed fires when a compaction attempt is abandoned
	// because the summarizing model call failed; the conversation is left
	// unchanged and compaction is retried after the next completed turn.
	ContextCompactionFailed()
	// ModelInfoUnavailable fires when the model's context window cannot be
	// fetched, leaving automatic compaction disabled. The fetch is retried
	// every turn until it succeeds, so the event fires on each failure.
	ModelInfoUnavailable()
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

// NewWriterFeedback returns a [Feedback] implementation that prints each event
// to w. [New]'s default is silent; install this with [Agent.SetFeedback] to opt
// into printing.
func NewWriterFeedback(w io.Writer) Feedback {
	return &writerFeedback{
		stdout: w,
	}
}

func (s *writerFeedback) ToolCalled(toolName string, args map[string]any) {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(s.stdout, "Tool called:", toolName)
		return
	}

	_, _ = fmt.Fprintln(s.stdout, "Tool called:", toolName, args)
}

func (s *writerFeedback) ContextCompacted() {
	_, _ = fmt.Fprintln(s.stdout, "Context was compacted")
}

func (s *writerFeedback) ContextCompactionFailed() {
	_, _ = fmt.Fprintln(s.stdout, "Context compaction failed")
}

func (s *writerFeedback) ModelInfoUnavailable() {
	_, _ = fmt.Fprintln(s.stdout, "Model info unavailable; automatic context compaction is disabled")
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

func (nullFeedback) ToolCalled(_ string, _ map[string]any) {
}

func (nullFeedback) ContextCompacted() {
}

func (nullFeedback) ContextCompactionFailed() {
}

func (nullFeedback) ModelInfoUnavailable() {
}

func (nullFeedback) SessionReset() {
}

func (nullFeedback) SessionStarted() {
}

func (nullFeedback) SessionClosed() {
}

package agent

import (
	"fmt"
	"io"
	"os"
	"time"
)

// Feedback receives an [Agent]'s events as they happen, so a caller can follow
// progress without affecting the conversation. Methods are called from inside
// [Agent.Process] and must not block. [New] installs a silent Feedback; pass
// [NewStdoutFeedback] to [Agent.SetFeedback] to print events.
type Feedback interface {
	// ToolCalled fires just before the agent runs the named tool, with the
	// arguments the model passed: nil when there are none, and with JSON types, so
	// numbers are float64. The tool runs with this map; do not modify it.
	ToolCalled(toolName string, args map[string]any)
	// ToolReturned fires just after the agent runs the named tool. Tools run one
	// at a time, so it always follows its [Feedback.ToolCalled]. result is what
	// the tool returned, empty when err is not nil. elapsed is how long the call
	// took.
	ToolReturned(toolName string, result string, err error, elapsed time.Duration)
	// InterimTextReceived fires when a response that asks for tools also has text,
	// before [Feedback.TokensUsed] for it. It never fires for empty text or for
	// the final answer, which is in [Response.Content].
	InterimTextReceived(content string)
	// ContextCompacted fires when the conversation is compacted (see
	// Config.CompactionThresholdPercent).
	ContextCompacted()
	// ContextCompactionFailed fires when the summarizing model call fails. The
	// conversation is left as it was, and compaction is tried again after the next
	// completed turn.
	ContextCompactionFailed()
	// ModelInfoUnavailable fires when the model's context window cannot be
	// fetched, which turns automatic compaction off. The fetch is retried every
	// turn, so it fires on each failure.
	ModelInfoUnavailable()
	// TokensUsed fires after each response that asks for tools, so token usage can
	// be tracked before the final answer. The final answer's usage is in
	// [Response.Metadata].
	TokensUsed(totalTokens int)
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

// NewStdoutFeedback returns a [Feedback] that prints each event to standard
// output. Install it with [Agent.SetFeedback].
func NewStdoutFeedback() Feedback {
	return NewWriterFeedback(os.Stdout)
}

// NewWriterFeedback returns a [Feedback] that prints each event to w. Install
// it with [Agent.SetFeedback].
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

func (s *writerFeedback) ToolReturned(toolName string, result string, err error,
	elapsed time.Duration,
) {
	if err != nil {
		_, _ = fmt.Fprintln(s.stdout, "Tool failed:", toolName, err, elapsed)
		return
	}

	_, _ = fmt.Fprintln(s.stdout, "Tool returned:", toolName, result, elapsed)
}

func (s *writerFeedback) InterimTextReceived(content string) {
	_, _ = fmt.Fprintln(s.stdout, "Interim text received:", content)
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

func (s *writerFeedback) TokensUsed(totalTokens int) {
	_, _ = fmt.Fprintln(s.stdout, "Tokens used:", totalTokens)
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

func (nullFeedback) ToolReturned(_ string, _ string, _ error, _ time.Duration) {
}

func (nullFeedback) InterimTextReceived(_ string) {
}

func (nullFeedback) ContextCompacted() {
}

func (nullFeedback) ContextCompactionFailed() {
}

func (nullFeedback) ModelInfoUnavailable() {
}

func (nullFeedback) TokensUsed(_ int) {
}

func (nullFeedback) SessionReset() {
}

func (nullFeedback) SessionStarted() {
}

func (nullFeedback) SessionClosed() {
}

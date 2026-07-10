package agent

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func exerciseAllEvents(fb Feedback) {
	fb.SessionStarted()
	fb.ToolCalled("echo")
	fb.ContextCompacted()
	fb.ContextCompactionFailed()
	fb.SessionReset()
	fb.SessionClosed()
}

func TestStdoutFeedback(t *testing.T) {
	t.Run("returns a usable sink whose events do not panic", func(t *testing.T) {
		// given
		fb := NewStdoutFeedback()
		// then
		require.NotNil(t, fb)
		// when / then: exercising every event must not panic
		require.NotPanics(t, func() { exerciseAllEvents(fb) })
	})
}

func TestDefaultFeedback(t *testing.T) {
	t.Run("silently ignores every event", func(t *testing.T) {
		// given
		var fb Feedback = nullFeedback{}
		// when / then: the noop sink must not panic
		require.NotPanics(t, func() { exerciseAllEvents(fb) })
	})
}

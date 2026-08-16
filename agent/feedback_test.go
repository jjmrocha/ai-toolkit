package agent

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func exerciseAllEvents(fb Feedback) {
	fb.SessionStarted()
	fb.ToolCalled("echo")
	fb.ContextCompacted()
	fb.ContextCompactionFailed()
	fb.ModelInfoUnavailable()
	fb.SessionReset()
	fb.SessionClosed()
}

func TestNewWriterFeedback(t *testing.T) {
	events := []struct {
		name     string
		fire     func(Feedback)
		expected string
	}{
		{
			name:     "session started",
			fire:     func(fb Feedback) { fb.SessionStarted() },
			expected: "New session started\n",
		},
		{
			name:     "tool called",
			fire:     func(fb Feedback) { fb.ToolCalled("get_weather") },
			expected: "Tool called: get_weather\n",
		},
		{
			name:     "context compacted",
			fire:     func(fb Feedback) { fb.ContextCompacted() },
			expected: "Context was compacted\n",
		},
		{
			name:     "context compaction failed",
			fire:     func(fb Feedback) { fb.ContextCompactionFailed() },
			expected: "Context compaction failed\n",
		},
		{
			name:     "model info unavailable",
			fire:     func(fb Feedback) { fb.ModelInfoUnavailable() },
			expected: "Model info unavailable; automatic context compaction is disabled\n",
		},
		{
			name:     "session reset",
			fire:     func(fb Feedback) { fb.SessionReset() },
			expected: "Session reset\n",
		},
		{
			name:     "session closed",
			fire:     func(fb Feedback) { fb.SessionClosed() },
			expected: "Session closed\n",
		},
	}

	for _, tc := range events {
		t.Run(tc.name, func(t *testing.T) {
			// given
			var out bytes.Buffer
			fb := NewWriterFeedback(&out)
			// when
			tc.fire(fb)
			// then
			result := out.String()
			assert.Equal(t, tc.expected, result)
		})
	}

	t.Run("writes one line per event, in the order they were fired", func(t *testing.T) {
		// given
		var out bytes.Buffer
		fb := NewWriterFeedback(&out)
		// when
		exerciseAllEvents(fb)
		// then
		result := out.String()
		expected := "New session started\n" +
			"Tool called: echo\n" +
			"Context was compacted\n" +
			"Context compaction failed\n" +
			"Model info unavailable; automatic context compaction is disabled\n" +
			"Session reset\n" +
			"Session closed\n"
		assert.Equal(t, expected, result)
	})
}

func TestNewStdoutFeedback(t *testing.T) {
	t.Run("returns a sink that prints to standard output", func(t *testing.T) {
		// when
		result := NewStdoutFeedback()
		// then
		sink, ok := result.(*writerFeedback)
		require.True(t, ok)
		assert.Same(t, os.Stdout, sink.stdout)
	})
}

func TestNullFeedback(t *testing.T) {
	t.Run("silently ignores every event", func(t *testing.T) {
		// given
		var fb Feedback = nullFeedback{}
		// when
		fireEvents := func() { exerciseAllEvents(fb) }
		// then: the sink writes nowhere, so surviving every event is all there is to observe
		assert.NotPanics(t, fireEvents)
	})
}

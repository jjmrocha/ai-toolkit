package agent

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func exerciseAllEvents(fb Feedback) {
	fb.SessionStarted()
	fb.SessionResumed("abc")
	fb.ToolCalled("echo", nil)
	fb.ToolReturned("echo", "ok", nil, time.Millisecond)
	fb.InterimTextReceived("checking")
	fb.ContextCompacted()
	fb.ContextCompactionFailed()
	fb.ModelInfoUnavailable()
	fb.TokensUsed(5)
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
			name:     "session resumed",
			fire:     func(fb Feedback) { fb.SessionResumed("abc") },
			expected: "Session resumed: abc\n",
		},
		{
			name:     "tool called",
			fire:     func(fb Feedback) { fb.ToolCalled("get_weather", map[string]any{"city": "Lisbon"}) },
			expected: "Tool called: get_weather map[city:Lisbon]\n",
		},
		{
			name:     "tool called without arguments",
			fire:     func(fb Feedback) { fb.ToolCalled("echo", nil) },
			expected: "Tool called: echo\n",
		},
		{
			name:     "interim text received",
			fire:     func(fb Feedback) { fb.InterimTextReceived("Let me check the config") },
			expected: "Interim text received: Let me check the config\n",
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
			name: "tokens used",
			fire: func(fb Feedback) {
				fb.TokensUsed(115)
			},
			expected: "Tokens used: 115\n",
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
			"Session resumed: abc\n" +
			"Tool called: echo\n" +
			"Tool returned: echo ok 1ms\n" +
			"Interim text received: checking\n" +
			"Context was compacted\n" +
			"Context compaction failed\n" +
			"Model info unavailable; automatic context compaction is disabled\n" +
			"Tokens used: 5\n" +
			"Session reset\n" +
			"Session closed\n"
		assert.Equal(t, expected, result)
	})
}

type sessionStartCounter struct {
	NopFeedback
	started int
}

func (c *sessionStartCounter) SessionStarted() {
	c.started++
}

func TestNopFeedback(t *testing.T) {
	t.Run("an embedding type receives the events it overrides", func(t *testing.T) {
		// given
		counter := &sessionStartCounter{}
		agt := agentWithLLM(&fakeLLM{}, counter, Config{})
		// when
		agt.StartSession(SessionConfig{Prompt: "p"})
		agt.Close()
		// then
		assert.Equal(t, 1, counter.started)
	})
}

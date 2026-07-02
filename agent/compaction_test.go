package agent

import (
	"testing"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/stretchr/testify/assert"
)

func turn(user, assistant string) []llm.Message {
	return []llm.Message{
		llm.UserMessage{Content: user},
		llm.AssistantMessage{Content: assistant},
	}
}

func TestCompactionThreshold(t *testing.T) {
	t.Run("applies the given percentage of the context window", func(t *testing.T) {
		// when
		result := compactionThreshold(8000, 90)
		// then
		assert.Equal(t, 7200, result)
	})

	t.Run("uses the default percentage when pct is zero", func(t *testing.T) {
		// when
		result := compactionThreshold(8000, 0)
		// then: defaultCompactionThresholdPercent is 85
		assert.Equal(t, 6800, result)
	})

	t.Run("returns the full window at one hundred percent", func(t *testing.T) {
		// when
		result := compactionThreshold(8000, 100)
		// then
		assert.Equal(t, 8000, result)
	})

	t.Run("returns zero for a zero context window", func(t *testing.T) {
		// when
		result := compactionThreshold(0, 90)
		// then
		assert.Equal(t, 0, result)
	})
}

func TestIndexOfTheBeginningOfTurnToKeep(t *testing.T) {
	// defaultKeepRecentTurns is 2, so the result is the start of the second-to-last
	// turn — the earliest message the two most recent turns keep.

	t.Run("keeps the last two turns, returning the earlier kept turn's user index", func(t *testing.T) {
		// given
		msgs := []llm.Message{llm.SystemMessage{Content: "sys"}}
		msgs = append(msgs, turn("u1", "a1")...)
		msgs = append(msgs, turn("u2", "a2")...)
		// when
		result := indexOfTheBeginningOfTurnToKeep(msgs)
		// then: keeping "u1" and "u2" starts at "u1", index 1
		assert.Equal(t, 1, result)
		assert.IsType(t, llm.UserMessage{}, msgs[result])
	})

	t.Run("keeps a tool-call turn when it is within the two most recent turns", func(t *testing.T) {
		// given
		msgs := []llm.Message{
			llm.SystemMessage{Content: "sys"},
			llm.UserMessage{Content: "u1"},
			llm.AssistantMessage{ToolCalls: []llm.ToolCall{{ID: "c1", Name: "tool"}}},
			llm.ToolMessage{ToolCallID: "c1", ToolName: "tool", Content: "result"},
			llm.AssistantMessage{Content: "a1"},
			llm.UserMessage{Content: "u2"},
			llm.AssistantMessage{Content: "a2"},
		}
		// when
		result := indexOfTheBeginningOfTurnToKeep(msgs)
		// then: the two kept turns start at "u1", index 1, so the tool pair is kept
		assert.Equal(t, 1, result)
	})

	t.Run("returns the only user-message index for a single turn", func(t *testing.T) {
		// given
		msgs := []llm.Message{llm.SystemMessage{Content: "sys"}}
		msgs = append(msgs, turn("u1", "a1")...)
		// when
		result := indexOfTheBeginningOfTurnToKeep(msgs)
		// then
		assert.Equal(t, 1, result)
	})

	t.Run("falls back to the last index when there is no user message", func(t *testing.T) {
		// given
		msgs := []llm.Message{llm.SystemMessage{Content: "sys"}}
		// when
		result := indexOfTheBeginningOfTurnToKeep(msgs)
		// then
		assert.Equal(t, 0, result)
	})

	t.Run("returns negative one for an empty conversation", func(t *testing.T) {
		// when
		result := indexOfTheBeginningOfTurnToKeep(nil)
		// then
		assert.Equal(t, -1, result)
	})
}

func TestRenderConversation(t *testing.T) {
	t.Run("renders each role and keeps tool calls and results visible", func(t *testing.T) {
		// given
		msgs := []llm.Message{
			llm.SystemMessage{Content: "be terse"},
			llm.UserMessage{Content: "weather?"},
			llm.AssistantMessage{
				Content:   "looking it up",
				ToolCalls: []llm.ToolCall{{Name: "get_weather", Arguments: map[string]any{"city": "Lisbon"}}},
			},
			llm.ToolMessage{ToolName: "get_weather", Content: "sunny"},
		}
		// when
		result := renderConversation(msgs)
		// then
		assert.Contains(t, result, "System: be terse")
		assert.Contains(t, result, "User: weather?")
		assert.Contains(t, result, "Assistant: looking it up")
		assert.Contains(t, result, "Assistant called tool get_weather")
		assert.Contains(t, result, "Tool get_weather result: sunny")
	})

	t.Run("returns an empty string for no messages", func(t *testing.T) {
		// when
		result := renderConversation(nil)
		// then
		assert.Empty(t, result)
	})
}

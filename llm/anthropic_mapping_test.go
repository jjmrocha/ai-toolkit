package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToAnthropicSystem(t *testing.T) {
	t.Run("returns empty when there are no system messages", func(t *testing.T) {
		// given
		messages := []Message{UserMessage{Content: "Hi"}}
		// when
		result := toAnthropicSystem(messages)
		// then
		assert.Empty(t, result)
	})

	t.Run("returns the single system message content", func(t *testing.T) {
		// given
		messages := []Message{SystemMessage{Content: "Be brief"}, UserMessage{Content: "Hi"}}
		// when
		result := toAnthropicSystem(messages)
		// then
		assert.Equal(t, "Be brief", result)
	})

	t.Run("joins multiple system messages with a blank line", func(t *testing.T) {
		// given
		messages := []Message{SystemMessage{Content: "Be brief"}, SystemMessage{Content: "Be kind"}}
		// when
		result := toAnthropicSystem(messages)
		// then
		assert.Equal(t, "Be brief\n\nBe kind", result)
	})
}

func TestToAnthropicMessages(t *testing.T) {
	t.Run("skips system messages", func(t *testing.T) {
		// given
		messages := []Message{SystemMessage{Content: "Be brief"}, UserMessage{Content: "Hi"}}
		// when
		result := toAnthropicMessages(messages)
		// then
		expected := []anthropicMessage{{Role: "user", Content: []anthropicContentBlock{{Type: "text", Text: "Hi"}}}}
		assert.Equal(t, expected, result)
	})

	t.Run("maps an assistant message with content and tool calls", func(t *testing.T) {
		// given
		messages := []Message{AssistantMessage{
			Content:   "Looking it up",
			ToolCalls: []ToolCall{{ID: "toolu_1", Name: "get_weather", Arguments: map[string]any{"city": "Lisbon"}}},
		}}
		// when
		result := toAnthropicMessages(messages)
		// then
		expected := []anthropicMessage{{Role: "assistant", Content: []anthropicContentBlock{
			{Type: "text", Text: "Looking it up"},
			{Type: "tool_use", ID: "toolu_1", Name: "get_weather", Input: map[string]any{"city": "Lisbon"}},
		}}}
		assert.Equal(t, expected, result)
	})

	t.Run("replays the raw content blocks when present", func(t *testing.T) {
		// given: tool_use before text, an order the rebuild path never produces
		raw := []anthropicContentBlock{
			{Type: "tool_use", ID: "toolu_1", Name: "get_weather", Input: map[string]any{"city": "Lisbon"}},
			{Type: "text", Text: "Looking it up"},
		}
		messages := []Message{AssistantMessage{
			Content:   "Looking it up",
			ToolCalls: []ToolCall{{ID: "toolu_1", Name: "get_weather", Arguments: map[string]any{"city": "Lisbon"}}},
			raw:       raw,
		}}
		// when
		result := toAnthropicMessages(messages)
		// then
		expected := []anthropicMessage{{Role: "assistant", Content: raw}}
		assert.Equal(t, expected, result)
	})

	t.Run("omits the text block when the assistant content is empty", func(t *testing.T) {
		// given
		messages := []Message{AssistantMessage{ToolCalls: []ToolCall{{ID: "toolu_1", Name: "ping"}}}}
		// when
		result := toAnthropicMessages(messages)
		// then
		expected := []anthropicMessage{{Role: "assistant", Content: []anthropicContentBlock{
			{Type: "tool_use", ID: "toolu_1", Name: "ping"},
		}}}
		assert.Equal(t, expected, result)
	})

	t.Run("maps a tool message onto a tool_result block in a user message", func(t *testing.T) {
		// given
		messages := []Message{ToolMessage{ToolCallID: "toolu_1", Content: "sunny"}}
		// when
		result := toAnthropicMessages(messages)
		// then
		expected := []anthropicMessage{{Role: "user", Content: []anthropicContentBlock{
			{Type: "tool_result", ToolUseID: "toolu_1", Content: "sunny"},
		}}}
		assert.Equal(t, expected, result)
	})

	t.Run("merges consecutive tool results into a single user message", func(t *testing.T) {
		// given
		messages := []Message{
			ToolMessage{ToolCallID: "toolu_1", Content: "sunny"},
			ToolMessage{ToolCallID: "toolu_2", Content: "warm"},
		}
		// when
		result := toAnthropicMessages(messages)
		// then
		expected := []anthropicMessage{{Role: "user", Content: []anthropicContentBlock{
			{Type: "tool_result", ToolUseID: "toolu_1", Content: "sunny"},
			{Type: "tool_result", ToolUseID: "toolu_2", Content: "warm"},
		}}}
		assert.Equal(t, expected, result)
	})

	t.Run("keeps alternating turns as separate messages", func(t *testing.T) {
		// given
		messages := []Message{
			UserMessage{Content: "Hi"},
			AssistantMessage{Content: "Hello"},
			UserMessage{Content: "Bye"},
		}
		// when
		result := toAnthropicMessages(messages)
		// then
		require.Len(t, result, 3)
		assert.Equal(t, "user", result[0].Role)
		assert.Equal(t, "assistant", result[1].Role)
		assert.Equal(t, "user", result[2].Role)
	})

	t.Run("empty input yields no messages", func(t *testing.T) {
		// when
		result := toAnthropicMessages(nil)
		// then
		assert.Empty(t, result)
	})
}

func TestToAnthropicThinking(t *testing.T) {
	t.Run("returns nil when effort is off", func(t *testing.T) {
		// when
		result := toAnthropicThinking(EffortOff)
		// then
		assert.Nil(t, result)
	})

	tests := []struct {
		name     string
		input    Effort
		expected *anthropicThinking
	}{
		{
			name:     "low effort enables thinking with the low budget",
			input:    EffortLow,
			expected: &anthropicThinking{Type: "enabled", BudgetTokens: 2000},
		},
		{
			name:     "medium effort enables thinking with the medium budget",
			input:    EffortMedium,
			expected: &anthropicThinking{Type: "enabled", BudgetTokens: 4000},
		},
		{
			name:     "max effort enables thinking with the max budget",
			input:    EffortMax,
			expected: &anthropicThinking{Type: "enabled", BudgetTokens: 16000},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// when
			result := toAnthropicThinking(tc.input)
			// then
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestToAnthropicTools(t *testing.T) {
	t.Run("nil tools yields nil", func(t *testing.T) {
		// when
		result := toAnthropicTools(nil)
		// then
		assert.Nil(t, result)
	})

	t.Run("empty tools yields nil", func(t *testing.T) {
		// when
		result := toAnthropicTools([]Tool{})
		// then
		assert.Nil(t, result)
	})

	t.Run("maps a tool onto an Anthropic tool definition", func(t *testing.T) {
		// given
		schema := map[string]any{"type": "object"}
		tools := []Tool{{Name: "get_weather", Description: "Get the weather", Schema: schema}}
		// when
		result := toAnthropicTools(tools)
		// then
		expected := []anthropicTool{{Name: "get_weather", Description: "Get the weather", InputSchema: schema}}
		assert.Equal(t, expected, result)
	})

	t.Run("maps every tool in order", func(t *testing.T) {
		// given
		tools := []Tool{{Name: "a"}, {Name: "b"}}
		// when
		result := toAnthropicTools(tools)
		// then
		require.Len(t, result, 2)
		assert.Equal(t, "a", result[0].Name)
		assert.Equal(t, "b", result[1].Name)
	})
}

func TestFromAnthropicToAssistantMessage(t *testing.T) {
	t.Run("concatenates text blocks and maps usage stats", func(t *testing.T) {
		// given
		resp := anthropicChatResponse{
			Content: []anthropicContentBlock{{Type: "text", Text: "Hello "}, {Type: "text", Text: "there"}},
			Usage:   anthropicUsage{InputTokens: 10, OutputTokens: 5},
		}
		// when
		result := fromAnthropicToAssistantMessage(resp)
		// then
		assert.Equal(t, "Hello there", result.Content)
		assert.Equal(t, Stats{PromptTokens: 10, OutputTokens: 5, TotalTokens: 15}, result.Stats)
	})

	t.Run("maps tool_use blocks onto tool calls", func(t *testing.T) {
		// given
		resp := anthropicChatResponse{Content: []anthropicContentBlock{
			{Type: "tool_use", ID: "toolu_1", Name: "get_weather", Input: map[string]any{"city": "Lisbon"}},
		}}
		// when
		result := fromAnthropicToAssistantMessage(resp)
		// then
		expected := ToolCall{ID: "toolu_1", Name: "get_weather", Arguments: map[string]any{"city": "Lisbon"}}
		require.Len(t, result.ToolCalls, 1)
		assert.Equal(t, expected, result.ToolCalls[0])
	})

	t.Run("leaves tool calls empty when the response has none", func(t *testing.T) {
		// given
		resp := anthropicChatResponse{Content: []anthropicContentBlock{{Type: "text", Text: "ok"}}}
		// when
		result := fromAnthropicToAssistantMessage(resp)
		// then
		assert.Empty(t, result.ToolCalls)
	})

	t.Run("preserves the raw content blocks for replay", func(t *testing.T) {
		// given
		resp := anthropicChatResponse{Content: []anthropicContentBlock{{Type: "text", Text: "ok"}}}
		// when
		result := fromAnthropicToAssistantMessage(resp)
		// then
		assert.Equal(t, resp.Content, result.raw)
	})
}

func TestFromAnthropicToModelInfo(t *testing.T) {
	t.Run("maps the display name and context size", func(t *testing.T) {
		// given
		model := anthropicModel{ID: "claude-opus-4-8", DisplayName: "Claude Opus 4.8", MaxInputTokens: 1000000}
		// when
		result, err := fromAnthropicToModelInfo(model)
		// then
		require.NoError(t, err)
		assert.Equal(t, &ModelInfo{Name: "Claude Opus 4.8", ContextSize: 1000000}, result)
	})

	t.Run("returns ErrMissingContextLength when no context size is reported", func(t *testing.T) {
		// given
		model := anthropicModel{ID: "claude-opus-4-8", DisplayName: "Claude Opus 4.8"}
		// when
		result, err := fromAnthropicToModelInfo(model)
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrMissingContextLength)
	})
}

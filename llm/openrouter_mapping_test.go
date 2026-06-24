package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToORMessages(t *testing.T) {
	tests := []struct {
		name     string
		input    []Message
		expected []orMessage
	}{
		{
			name:     "system message",
			input:    []Message{SystemMessage{Content: "Be brief"}},
			expected: []orMessage{{Role: "system", Content: "Be brief"}},
		},
		{
			name:     "user message",
			input:    []Message{UserMessage{Content: "Hi"}},
			expected: []orMessage{{Role: "user", Content: "Hi"}},
		},
		{
			name:     "tool message",
			input:    []Message{ToolMessage{ToolCallID: "call_1", Content: "sunny"}},
			expected: []orMessage{{Role: "tool", Content: "sunny", ToolCallID: "call_1"}},
		},
		{
			name:     "assistant message without tool calls",
			input:    []Message{AssistantMessage{Content: "done"}},
			expected: []orMessage{{Role: "assistant", Content: "done"}},
		},
		{
			name: "assistant message with tool calls serializes arguments as a JSON string",
			input: []Message{AssistantMessage{ToolCalls: []ToolCall{
				{ID: "call_1", Name: "get_weather", Arguments: map[string]any{"city": "Lisbon"}},
			}}},
			expected: []orMessage{{
				Role: "assistant",
				ToolCalls: []orToolCall{{
					ID:       "call_1",
					Type:     "function",
					Function: orToolCallFunction{Name: "get_weather", Arguments: `{"city":"Lisbon"}`},
				}},
			}},
		},
		{
			name: "preserves order across multiple messages",
			input: []Message{
				SystemMessage{Content: "Be brief"},
				UserMessage{Content: "Hi"},
			},
			expected: []orMessage{
				{Role: "system", Content: "Be brief"},
				{Role: "user", Content: "Hi"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// given
			input := tc.input
			// when
			result, err := toORMessages(input)
			// then
			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}

	t.Run("empty input yields no messages", func(t *testing.T) {
		// when
		result, err := toORMessages(nil)
		// then
		require.NoError(t, err)
		assert.Empty(t, result)
	})
}

func TestToORTools(t *testing.T) {
	t.Run("nil tools yields nil", func(t *testing.T) {
		// when
		result := toORTools(nil)
		// then
		assert.Nil(t, result)
	})

	t.Run("empty tools yields nil", func(t *testing.T) {
		// when
		result := toORTools([]Tool{})
		// then
		assert.Nil(t, result)
	})

	t.Run("maps a tool onto an OpenAI function definition", func(t *testing.T) {
		// given
		schema := map[string]any{"type": "object"}
		tools := []Tool{{Name: "get_weather", Description: "Get the weather", Schema: schema}}
		// when
		result := toORTools(tools)
		// then
		expected := []orTool{{
			Type:     "function",
			Function: orToolFunction{Name: "get_weather", Description: "Get the weather", Parameters: schema},
		}}
		assert.Equal(t, expected, result)
	})

	t.Run("maps every tool in order", func(t *testing.T) {
		// given
		tools := []Tool{{Name: "a"}, {Name: "b"}}
		// when
		result := toORTools(tools)
		// then
		require.Len(t, result, 2)
		assert.Equal(t, "a", result[0].Function.Name)
		assert.Equal(t, "b", result[1].Function.Name)
	})
}

func TestFromORToAssistantMessage(t *testing.T) {
	t.Run("maps content and usage stats", func(t *testing.T) {
		// given
		resp := orChatResponse{
			Choices: []orChoice{{Message: orResponseMessage{Content: "Hello there"}}},
			Usage:   orUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
		}
		// when
		result, err := fromORToAssistantMessage(resp)
		// then
		require.NoError(t, err)
		assert.Equal(t, "Hello there", result.Content)
		assert.Equal(t, Stats{PromptTokens: 10, OutputTokens: 5, TotalTokens: 15}, result.Stats)
	})

	t.Run("decodes tool call arguments from a JSON string", func(t *testing.T) {
		// given
		resp := orChatResponse{Choices: []orChoice{{Message: orResponseMessage{ToolCalls: []orToolCall{{
			ID:       "call_1",
			Function: orToolCallFunction{Name: "get_weather", Arguments: `{"city":"Lisbon"}`},
		}}}}}}
		// when
		result, err := fromORToAssistantMessage(resp)
		// then
		require.NoError(t, err)
		expected := ToolCall{ID: "call_1", Name: "get_weather", Arguments: map[string]any{"city": "Lisbon"}}
		assert.Equal(t, expected, result.ToolCalls[0])
	})

	t.Run("leaves arguments nil when the tool call has none", func(t *testing.T) {
		// given
		resp := orChatResponse{Choices: []orChoice{{Message: orResponseMessage{ToolCalls: []orToolCall{{
			ID:       "call_1",
			Function: orToolCallFunction{Name: "ping", Arguments: ""},
		}}}}}}
		// when
		result, err := fromORToAssistantMessage(resp)
		// then
		require.NoError(t, err)
		assert.Nil(t, result.ToolCalls[0].Arguments)
	})

	t.Run("returns an error when tool call arguments are malformed", func(t *testing.T) {
		// given
		resp := orChatResponse{Choices: []orChoice{{Message: orResponseMessage{ToolCalls: []orToolCall{{
			ID:       "call_1",
			Function: orToolCallFunction{Name: "get_weather", Arguments: "{not json"},
		}}}}}}
		// when
		result, err := fromORToAssistantMessage(resp)
		// then
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "decoding tool call arguments")
	})
}

func TestFromORToModelInfo(t *testing.T) {
	models := []orModel{
		{ID: "anthropic/claude-3", Name: "Claude 3", ContextLength: 200000},
		{ID: "openai/gpt-4o", Name: "OpenAI: GPT-4o", ContextLength: 128000},
	}

	t.Run("maps the matching model", func(t *testing.T) {
		// when
		result, err := fromORToModelInfo(models, "openai/gpt-4o")
		// then
		require.NoError(t, err)
		assert.Equal(t, ModelInfo{Name: "OpenAI: GPT-4o", ContextSize: 128000}, result)
	})

	t.Run("returns ErrModelNotFound when the id is absent", func(t *testing.T) {
		// when
		result, err := fromORToModelInfo(models, "missing/model")
		// then
		assert.Equal(t, ModelInfo{}, result)
		assert.ErrorIs(t, err, ErrModelNotFound)
	})

	t.Run("returns ErrModelNotFound for an empty list", func(t *testing.T) {
		// when
		result, err := fromORToModelInfo(nil, "openai/gpt-4o")
		// then
		assert.Equal(t, ModelInfo{}, result)
		assert.ErrorIs(t, err, ErrModelNotFound)
	})
}

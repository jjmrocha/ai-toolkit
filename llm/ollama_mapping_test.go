package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToOllamaMessages(t *testing.T) {
	tests := []struct {
		name     string
		input    []Message
		expected []ollamaMessage
	}{
		{
			name:     "system message",
			input:    []Message{SystemMessage{Content: "Be brief"}},
			expected: []ollamaMessage{{Role: "system", Content: "Be brief"}},
		},
		{
			name:     "user message",
			input:    []Message{UserMessage{Content: "Hi"}},
			expected: []ollamaMessage{{Role: "user", Content: "Hi"}},
		},
		{
			name:     "tool message correlates by tool name, not call id",
			input:    []Message{ToolMessage{ToolCallID: "ignored", ToolName: "get_weather", Content: "sunny"}},
			expected: []ollamaMessage{{Role: "tool", Content: "sunny", ToolName: "get_weather"}},
		},
		{
			name: "assistant tool call keeps arguments as an object and drops the id",
			input: []Message{AssistantMessage{ToolCalls: []ToolCall{
				{ID: "call_1", Name: "get_weather", Arguments: map[string]any{"city": "Tokyo"}},
			}}},
			expected: []ollamaMessage{{
				Role: "assistant",
				ToolCalls: []ollamaToolCall{{
					Function: ollamaToolCallFunction{Name: "get_weather", Arguments: map[string]any{"city": "Tokyo"}},
				}},
			}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// when
			result := toOllamaMessages(tc.input)
			// then
			assert.Equal(t, tc.expected, result)
		})
	}

	t.Run("empty input yields no messages", func(t *testing.T) {
		// when
		result := toOllamaMessages(nil)
		// then
		assert.Empty(t, result)
	})
}

func TestToOllamaThink(t *testing.T) {
	tests := []struct {
		name     string
		input    Effort
		expected any
	}{
		{name: "off disables thinking", input: EffortOff, expected: false},
		{name: "low maps to the low level", input: EffortLow, expected: "low"},
		{name: "medium maps to the medium level", input: EffortMedium, expected: "medium"},
		{name: "max maps to high, the strongest level Ollama accepts", input: EffortMax, expected: "high"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// when
			result := toOllamaThink(tc.input)
			// then
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestToOllamaTools(t *testing.T) {
	t.Run("nil tools yields nil", func(t *testing.T) {
		assert.Nil(t, toOllamaTools(nil))
	})

	t.Run("maps a tool onto a function definition", func(t *testing.T) {
		// given
		schema := map[string]any{"type": "object"}
		tools := []Tool{{Name: "get_weather", Description: "Get the weather", Schema: schema}}
		// when
		result := toOllamaTools(tools)
		// then
		expected := []ollamaTool{{
			Type:     "function",
			Function: ollamaToolFunction{Name: "get_weather", Description: "Get the weather", Parameters: schema},
		}}
		assert.Equal(t, expected, result)
	})
}

func TestFromOllamaToAssistantMessage(t *testing.T) {
	t.Run("maps content and sums prompt and eval token counts", func(t *testing.T) {
		// given
		resp := ollamaChatResponse{
			Message:         ollamaResponseMessage{Content: "Hello there"},
			PromptEvalCount: 10,
			EvalCount:       5,
		}
		// when
		result := fromOllamaToAssistantMessage(resp)
		// then
		assert.Equal(t, "Hello there", result.Content)
		assert.Equal(t, Stats{PromptTokens: 10, OutputTokens: 5, TotalTokens: 15}, result.Stats)
	})

	t.Run("maps tool calls with object arguments and no id", func(t *testing.T) {
		// given
		resp := ollamaChatResponse{Message: ollamaResponseMessage{ToolCalls: []ollamaToolCall{{
			Function: ollamaToolCallFunction{Name: "get_weather", Arguments: map[string]any{"city": "Tokyo"}},
		}}}}
		// when
		result := fromOllamaToAssistantMessage(resp)
		// then
		expected := ToolCall{Name: "get_weather", Arguments: map[string]any{"city": "Tokyo"}}
		require.Len(t, result.ToolCalls, 1)
		assert.Equal(t, expected, result.ToolCalls[0])
	})
}

func TestFromOllamaToModelInfo(t *testing.T) {
	t.Run("reads the architecture-prefixed context length", func(t *testing.T) {
		// given
		resp := ollamaShowResponse{ModelInfo: map[string]any{
			"general.architecture": "llama",
			"llama.context_length": float64(8192),
		}}
		// when
		result, err := fromOllamaToModelInfo(resp, "llama3.2")
		// then
		require.NoError(t, err)
		assert.Equal(t, &ModelInfo{Name: "llama3.2", ContextSize: 8192}, result)
	})

	t.Run("returns an error when the context length key is absent", func(t *testing.T) {
		// given
		resp := ollamaShowResponse{ModelInfo: map[string]any{"general.architecture": "llama"}}
		// when
		_, err := fromOllamaToModelInfo(resp, "llama3.2")
		// then
		assert.ErrorIs(t, err, ErrMissingContextLength)
	})

	t.Run("returns an error when the architecture key is absent", func(t *testing.T) {
		// given
		resp := ollamaShowResponse{ModelInfo: map[string]any{}}
		// when
		_, err := fromOllamaToModelInfo(resp, "llama3.2")
		// then
		assert.ErrorIs(t, err, ErrMissingContextLength)
	})

	t.Run("returns an error when the context length is zero", func(t *testing.T) {
		// given
		resp := ollamaShowResponse{ModelInfo: map[string]any{
			"general.architecture": "llama",
			"llama.context_length": float64(0),
		}}
		// when
		_, err := fromOllamaToModelInfo(resp, "llama3.2")
		// then
		assert.ErrorIs(t, err, ErrMissingContextLength)
	})
}

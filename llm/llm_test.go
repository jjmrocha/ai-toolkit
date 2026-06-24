package llm

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	errorCases := []struct {
		name        string
		cfg         Config
		expectedErr error
	}{
		{
			name:        "missing provider",
			cfg:         Config{Model: "openai/gpt-4o"},
			expectedErr: ErrMissingProvider,
		},
		{
			name:        "missing model",
			cfg:         Config{Provider: ProviderOpenRouter},
			expectedErr: ErrMissingModel,
		},
		{
			name:        "unsupported provider",
			cfg:         Config{Provider: "bogus", Model: "openai/gpt-4o"},
			expectedErr: ErrUnsupportedProvider,
		},
		{
			name:        "provider construction error propagates",
			cfg:         Config{Provider: ProviderOpenRouter, Model: "openai/gpt-4o"}, // no API key
			expectedErr: ErrMissingAPIKey,
		},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			// when
			result, err := New(tc.cfg)
			// then
			assert.Nil(t, result)
			assert.ErrorIs(t, err, tc.expectedErr)
		})
	}

	t.Run("valid openrouter config returns a configured LLM", func(t *testing.T) {
		// given
		cfg := Config{Provider: ProviderOpenRouter, Model: "openai/gpt-4o", APIKey: "sk-test"}
		// when
		result, err := New(cfg)
		// then
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, cfg, result.cfg)
	})

	t.Run("valid ollama config returns a configured LLM without an API key", func(t *testing.T) {
		// given
		cfg := Config{Provider: ProviderOllama, Model: "llama3.2"}
		// when
		result, err := New(cfg)
		// then
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, cfg, result.cfg)
	})
}

func TestLLMChat(t *testing.T) {
	t.Run("delegates to the provider and returns its result", func(t *testing.T) {
		// given
		expected := &AssistantMessage{Content: "Hello there"}
		var gotMessages []Message
		var gotTools []Tool
		llm := &LLM{provider: fakeProvider{
			chatFunc: func(_ context.Context, messages []Message, tools []Tool) (*AssistantMessage, error) {
				gotMessages = messages
				gotTools = tools
				return expected, nil
			},
		}}
		messages := []Message{UserMessage{Content: "Hi"}}
		tools := []Tool{{Name: "get_weather"}}
		// when
		result, err := llm.Chat(t.Context(), messages, tools)
		// then
		require.NoError(t, err)
		assert.Same(t, expected, result)
		assert.Equal(t, messages, gotMessages)
		assert.Equal(t, tools, gotTools)
	})

	t.Run("propagates the provider error", func(t *testing.T) {
		// given
		expectedErr := errors.New("boom")
		llm := &LLM{provider: fakeProvider{
			chatFunc: func(context.Context, []Message, []Tool) (*AssistantMessage, error) {
				return nil, expectedErr
			},
		}}
		// when
		result, err := llm.Chat(t.Context(), nil, nil)
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, expectedErr)
	})
}

func TestLLMModelInfo(t *testing.T) {
	t.Run("delegates to the provider and returns its result", func(t *testing.T) {
		// given
		expected := ModelInfo{Name: "OpenAI: GPT-4o", ContextSize: 128000}
		llm := &LLM{provider: fakeProvider{
			modelInfoFunc: func(context.Context) (ModelInfo, error) {
				return expected, nil
			},
		}}
		// when
		result, err := llm.ModelInfo(t.Context())
		// then
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("propagates the provider error", func(t *testing.T) {
		// given
		expectedErr := errors.New("boom")
		llm := &LLM{provider: fakeProvider{
			modelInfoFunc: func(context.Context) (ModelInfo, error) {
				return ModelInfo{}, expectedErr
			},
		}}
		// when
		result, err := llm.ModelInfo(t.Context())
		// then
		assert.Equal(t, ModelInfo{}, result)
		assert.ErrorIs(t, err, expectedErr)
	})
}

// fakeProvider is an in-package llmProvider double whose behavior is set
// per-test via function fields.
type fakeProvider struct {
	chatFunc      func(context.Context, []Message, []Tool) (*AssistantMessage, error)
	modelInfoFunc func(context.Context) (ModelInfo, error)
}

func (f fakeProvider) chat(ctx context.Context, messages []Message, tools []Tool) (*AssistantMessage, error) {
	return f.chatFunc(ctx, messages, tools)
}

func (f fakeProvider) modelInfo(ctx context.Context) (ModelInfo, error) {
	return f.modelInfoFunc(ctx)
}

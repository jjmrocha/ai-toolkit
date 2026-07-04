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
		{
			name:        "unknown effort is rejected",
			cfg:         Config{Provider: ProviderOllama, Model: "llama3.2", Effort: "ultra"},
			expectedErr: ErrInvalidEffort,
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
		cfg.Effort = EffortOff                 // New defaults an unset effort
		cfg.Models = []string{"openai/gpt-4o"} // New seeds the active model
		assert.Equal(t, cfg, result.config)
	})

	t.Run("valid ollama config returns a configured LLM without an API key", func(t *testing.T) {
		// given
		cfg := Config{Provider: ProviderOllama, Model: "llama3.2"}
		// when
		result, err := New(cfg)
		// then
		require.NoError(t, err)
		require.NotNil(t, result)
		cfg.Effort = EffortOff            // New defaults an unset effort
		cfg.Models = []string{"llama3.2"} // New seeds the active model
		assert.Equal(t, cfg, result.config)
	})

	t.Run("valid anthropic config returns a configured LLM", func(t *testing.T) {
		// given
		cfg := Config{Provider: ProviderAnthropic, Model: "claude-opus-4-8", APIKey: "sk-test"}
		// when
		result, err := New(cfg)
		// then
		require.NoError(t, err)
		require.NotNil(t, result)
		cfg.Effort = EffortOff                   // New defaults an unset effort
		cfg.Models = []string{"claude-opus-4-8"} // New seeds the active model
		assert.Equal(t, cfg, result.config)
	})

	t.Run("seeds Models with the active model when it is absent", func(t *testing.T) {
		// given: Models does not list the configured Model
		cfg := Config{Provider: ProviderOllama, Model: "llama3.2", Models: []string{"other"}}
		// when
		result, err := New(cfg)
		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"llama3.2", "other"}, result.AvailableModels())
		assert.NoError(t, result.ChangeModel("llama3.2")) // the active model is switchable
	})

	t.Run("does not duplicate the active model when Models already lists it", func(t *testing.T) {
		// given
		cfg := Config{Provider: ProviderOllama, Model: "llama3.2", Models: []string{"llama3.2", "other"}}
		// when
		result, err := New(cfg)
		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"llama3.2", "other"}, result.AvailableModels())
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
		expected := &ModelInfo{Name: "OpenAI: GPT-4o", ContextSize: 128000}
		llm := &LLM{provider: fakeProvider{
			modelInfoFunc: func(context.Context) (*ModelInfo, error) {
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
			modelInfoFunc: func(context.Context) (*ModelInfo, error) {
				return nil, expectedErr
			},
		}}
		// when
		result, err := llm.ModelInfo(t.Context())
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, expectedErr)
	})
}

func TestLLMCurrentModel(t *testing.T) {
	t.Run("delegates to the provider", func(t *testing.T) {
		// given
		llm := &LLM{provider: fakeProvider{
			currentModelFunc: func() string { return "m1" },
		}}
		// when
		result := llm.CurrentModel()
		// then
		assert.Equal(t, "m1", result)
	})
}

func TestLLMAvailableModels(t *testing.T) {
	t.Run("returns the configured model list", func(t *testing.T) {
		// given
		expected := []string{"m1", "m2"}
		llm := &LLM{config: Config{Models: expected}}
		// when
		result := llm.AvailableModels()
		// then
		assert.Equal(t, expected, result)
	})

	t.Run("returns nil when no models are configured", func(t *testing.T) {
		// given
		llm := &LLM{}
		// when
		result := llm.AvailableModels()
		// then
		assert.Nil(t, result)
	})
}

func TestLLMChangeModel(t *testing.T) {
	t.Run("switches to a configured model via the provider", func(t *testing.T) {
		// given
		var changed string
		llm := &LLM{
			config: Config{Models: []string{"m1", "m2"}},
			provider: fakeProvider{
				changeModelFunc: func(m string) error { changed = m; return nil },
			},
		}
		// when
		err := llm.ChangeModel("m2")
		// then
		require.NoError(t, err)
		assert.Equal(t, "m2", changed)
	})

	t.Run("returns ErrMissingModel when model is empty", func(t *testing.T) {
		// given
		llm := &LLM{config: Config{Models: []string{"m1"}}}
		// when
		err := llm.ChangeModel("")
		// then
		assert.ErrorIs(t, err, ErrMissingModel)
	})

	t.Run("returns ErrModelNotFound when model is not in the configured list", func(t *testing.T) {
		// given
		llm := &LLM{config: Config{Models: []string{"m1"}}}
		// when
		err := llm.ChangeModel("m2")
		// then
		assert.ErrorIs(t, err, ErrModelNotFound)
	})
}

type fakeProvider struct {
	chatFunc         func(context.Context, []Message, []Tool) (*AssistantMessage, error)
	modelInfoFunc    func(context.Context) (*ModelInfo, error)
	changeModelFunc  func(string) error
	currentModelFunc func() string
	effortFunc       func() Effort
	changeEffortFunc func(Effort)
}

func (f fakeProvider) chat(ctx context.Context, messages []Message, tools []Tool) (*AssistantMessage, error) {
	return f.chatFunc(ctx, messages, tools)
}

func (f fakeProvider) modelInfo(ctx context.Context) (*ModelInfo, error) {
	return f.modelInfoFunc(ctx)
}

func (f fakeProvider) changeModel(model string) error {
	if f.changeModelFunc == nil {
		return nil
	}
	return f.changeModelFunc(model)
}

func (f fakeProvider) currentModel() string {
	if f.currentModelFunc == nil {
		return ""
	}
	return f.currentModelFunc()
}

func (f fakeProvider) effort() Effort {
	if f.effortFunc == nil {
		return EffortOff
	}
	return f.effortFunc()
}

func (f fakeProvider) changeEffort(e Effort) {
	if f.changeEffortFunc == nil {
		return
	}
	f.changeEffortFunc(e)
}

package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
			cfg:         Config{Provider: ProviderOpenRouter, Model: "openai/gpt-4o"},
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
			result, err := New(t.Context(), tc.cfg)
			// then
			assert.Nil(t, result)
			assert.ErrorIs(t, err, tc.expectedErr)
		})
	}

	t.Run("returns the provider error when the model info cannot be fetched", func(t *testing.T) {
		// given
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(server.Close)
		cfg := Config{Provider: ProviderOllama, Model: "llama3.2", BaseURL: server.URL}
		// when
		result, err := New(t.Context(), cfg)
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrModelNotFound)
	})

	t.Run("valid openrouter config returns a configured LLM", func(t *testing.T) {
		// given
		expected := &ModelInfo{Provider: ProviderOpenRouter, Name: "openai/gpt-4o", ContextSize: 128000}
		url := newModelInfoServer(t, `{"data":[{"id":"openai/gpt-4o","context_length":128000}]}`)
		cfg := Config{Provider: ProviderOpenRouter, Model: "openai/gpt-4o", APIKey: "sk-test", BaseURL: url}
		// when
		result, err := New(t.Context(), cfg)
		// then
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "openai/gpt-4o", result.CurrentModel())
		assert.Equal(t, EffortOff, result.Effort())
		assert.Equal(t, []string{"openai/gpt-4o"}, result.AvailableModels())
		assert.Equal(t, expected, result.ModelInfo())
	})

	t.Run("valid ollama config returns a configured LLM without an API key", func(t *testing.T) {
		// given
		expected := &ModelInfo{Provider: ProviderOllama, Name: "llama3.2", ContextSize: 8192}
		url := newModelInfoServer(t, `{"model_info":{"general.architecture":"llama","llama.context_length":8192}}`)
		cfg := Config{Provider: ProviderOllama, Model: "llama3.2", BaseURL: url}
		// when
		result, err := New(t.Context(), cfg)
		// then
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "llama3.2", result.CurrentModel())
		assert.Equal(t, EffortOff, result.Effort())
		assert.Equal(t, []string{"llama3.2"}, result.AvailableModels())
		assert.Equal(t, expected, result.ModelInfo())
	})

	t.Run("valid anthropic config returns a configured LLM", func(t *testing.T) {
		// given
		expected := &ModelInfo{Provider: ProviderAnthropic, Name: "claude-opus-4-8", ContextSize: 1000000}
		url := newModelInfoServer(t, `{"id":"claude-opus-4-8","max_input_tokens":1000000}`)
		cfg := Config{Provider: ProviderAnthropic, Model: "claude-opus-4-8", APIKey: "sk-test", BaseURL: url}
		// when
		result, err := New(t.Context(), cfg)
		// then
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "claude-opus-4-8", result.CurrentModel())
		assert.Equal(t, EffortOff, result.Effort())
		assert.Equal(t, []string{"claude-opus-4-8"}, result.AvailableModels())
		assert.Equal(t, expected, result.ModelInfo())
	})

	t.Run("seeds Models with the active model when it is absent", func(t *testing.T) {
		// given
		url := newModelInfoServer(t, `{"model_info":{"general.architecture":"llama","llama.context_length":8192}}`)
		cfg := Config{Provider: ProviderOllama, Model: "llama3.2", Models: []string{"other"}, BaseURL: url}
		// when
		result, err := New(t.Context(), cfg)
		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"llama3.2", "other"}, result.AvailableModels())
		_, err = result.ChangeModel(t.Context(), "llama3.2")
		assert.NoError(t, err)
	})

	t.Run("does not duplicate the active model when Models already lists it", func(t *testing.T) {
		// given
		url := newModelInfoServer(t, `{"model_info":{"general.architecture":"llama","llama.context_length":8192}}`)
		cfg := Config{Provider: ProviderOllama, Model: "llama3.2", Models: []string{"llama3.2", "other"}, BaseURL: url}
		// when
		result, err := New(t.Context(), cfg)
		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"llama3.2", "other"}, result.AvailableModels())
	})
}

func TestLLMChat(t *testing.T) {
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
	t.Run("returns the info fetched when the client was created", func(t *testing.T) {
		// given
		expected := &ModelInfo{Provider: ProviderOllama, Name: "m1", ContextSize: 1000}
		llm := &LLM{modelInfo: &ModelInfo{Provider: ProviderOllama, Name: "m1", ContextSize: 1000}}
		// when
		result := llm.ModelInfo()
		// then
		assert.Equal(t, expected, result)
	})

	t.Run("returns a copy that callers cannot use to change the stored info", func(t *testing.T) {
		// given
		expected := &ModelInfo{Name: "m1", ContextSize: 1000}
		llm := &LLM{modelInfo: &ModelInfo{Name: "m1", ContextSize: 1000}}
		llm.ModelInfo().ContextSize = 1
		// when
		result := llm.ModelInfo()
		// then
		assert.Equal(t, expected, result)
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

	t.Run("returns a copy the caller cannot use to widen the list", func(t *testing.T) {
		// given
		models := []string{"llama3.2", "other"}
		url := newModelInfoServer(t, `{"model_info":{"general.architecture":"llama","llama.context_length":8192}}`)
		client, err := New(t.Context(), Config{Provider: ProviderOllama, Model: "llama3.2", Models: models, BaseURL: url})
		require.NoError(t, err)
		client.AvailableModels()[0] = "x"
		models[1] = "y"
		// when
		_, err = client.ChangeModel(t.Context(), "x")
		// then
		assert.ErrorIs(t, err, ErrModelNotFound)
		assert.Equal(t, []string{"llama3.2", "other"}, client.AvailableModels())
	})
}

func TestLLMChangeModel(t *testing.T) {
	t.Run("switches to a configured model via the provider and returns its info", func(t *testing.T) {
		// given
		var changed string
		expected := &ModelInfo{Name: "m2", ContextSize: 2000}
		llm := &LLM{
			config:    Config{Models: []string{"m1", "m2"}},
			modelInfo: &ModelInfo{Name: "m1", ContextSize: 1000},
			provider: fakeProvider{
				changeModelFunc: func(_ context.Context, m string) (*ModelInfo, error) {
					changed = m
					return &ModelInfo{Name: "m2", ContextSize: 2000}, nil
				},
			},
		}
		// when
		result, err := llm.ChangeModel(t.Context(), "m2")
		// then
		require.NoError(t, err)
		assert.Equal(t, "m2", changed)
		assert.Equal(t, expected, result)
		assert.Equal(t, expected, llm.ModelInfo())
	})

	t.Run("keeps the current info when the provider fails", func(t *testing.T) {
		// given
		expectedErr := errors.New("boom")
		expected := &ModelInfo{Name: "m1", ContextSize: 1000}
		llm := &LLM{
			config:    Config{Models: []string{"m1", "m2"}},
			modelInfo: &ModelInfo{Name: "m1", ContextSize: 1000},
			provider: fakeProvider{
				changeModelFunc: func(context.Context, string) (*ModelInfo, error) {
					return nil, expectedErr
				},
			},
		}
		// when
		result, err := llm.ChangeModel(t.Context(), "m2")
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, expectedErr)
		assert.Equal(t, expected, llm.ModelInfo())
	})

	t.Run("returns ErrMissingModel when model is empty", func(t *testing.T) {
		// given
		llm := &LLM{config: Config{Models: []string{"m1"}}}
		// when
		result, err := llm.ChangeModel(t.Context(), "")
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrMissingModel)
	})

	t.Run("returns ErrModelNotFound when model is not in the configured list", func(t *testing.T) {
		// given
		llm := &LLM{config: Config{Models: []string{"m1"}}}
		// when
		result, err := llm.ChangeModel(t.Context(), "m2")
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrModelNotFound)
	})
}

func TestLLMChangeEffort(t *testing.T) {
	t.Run("sets the effort via the provider", func(t *testing.T) {
		// given
		var changed Effort
		llm := &LLM{
			provider: fakeProvider{
				changeEffortFunc: func(e Effort) { changed = e },
			},
		}
		// when
		err := llm.ChangeEffort(EffortMax)
		// then
		require.NoError(t, err)
		assert.Equal(t, EffortMax, changed)
	})

	t.Run("treats an empty effort as EffortOff", func(t *testing.T) {
		// given
		var changed Effort
		llm := &LLM{
			provider: fakeProvider{
				changeEffortFunc: func(e Effort) { changed = e },
			},
		}
		// when
		err := llm.ChangeEffort("")
		// then
		require.NoError(t, err)
		assert.Equal(t, EffortOff, changed)
	})

	t.Run("returns ErrInvalidEffort and leaves the effort alone", func(t *testing.T) {
		// given
		called := false
		llm := &LLM{
			provider: fakeProvider{
				changeEffortFunc: func(Effort) { called = true },
			},
		}
		// when
		err := llm.ChangeEffort(Effort("bogus"))
		// then
		assert.ErrorIs(t, err, ErrInvalidEffort)
		assert.False(t, called)
	})
}

type fakeProvider struct {
	chatFunc         func(context.Context, []Message, []Tool) (*AssistantMessage, error)
	modelInfoFunc    func(context.Context) (*ModelInfo, error)
	changeModelFunc  func(context.Context, string) (*ModelInfo, error)
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

func (f fakeProvider) changeModel(ctx context.Context, model string) (*ModelInfo, error) {
	return f.changeModelFunc(ctx, model)
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

func newModelInfoServer(t testing.TB, body string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, body)
	}))
	t.Cleanup(server.Close)

	return server.URL
}

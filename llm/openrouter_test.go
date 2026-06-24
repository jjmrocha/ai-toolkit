package llm

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOpenRouter(t *testing.T) {
	t.Run("returns error when API key is missing", func(t *testing.T) {
		// when
		_, err := newOpenRouter(Config{Model: "openai/gpt-4o"})
		// then
		assert.ErrorIs(t, err, ErrMissingAPIKey)
	})

	t.Run("applies the default base URL when none is provided", func(t *testing.T) {
		// when
		result, err := newOpenRouter(Config{APIKey: "sk-test", Model: "openai/gpt-4o"})
		// then
		require.NoError(t, err)
		assert.Equal(t, defaultBaseURL, result.cfg.BaseURL)
	})

	t.Run("keeps the configured base URL", func(t *testing.T) {
		// given
		baseURL := "https://proxy.example.com/api/v1"
		// when
		result, err := newOpenRouter(Config{APIKey: "sk-test", Model: "openai/gpt-4o", BaseURL: baseURL})
		// then
		require.NoError(t, err)
		assert.Equal(t, baseURL, result.cfg.BaseURL)
	})
}

func TestChat(t *testing.T) {
	t.Run("sends a POST request carrying auth, model, messages and tools", func(t *testing.T) {
		// given
		var (
			gotMethod string
			gotPath   string
			gotAuth   string
			gotBody   []byte
		)
		o := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotPath = r.URL.Path
			gotAuth = r.Header.Get("Authorization")
			gotBody, _ = io.ReadAll(r.Body)
			writeJSON(t, w, `{"choices":[{"message":{"content":"ok"}}],"usage":{}}`)
		})
		messages := []Message{SystemMessage{Content: "Be brief"}, UserMessage{Content: "Hi"}}
		tools := []Tool{{Name: "get_weather", Description: "Get the weather", Schema: map[string]any{"type": "object"}}}
		// when
		_, err := o.chat(t.Context(), messages, tools)
		// then
		require.NoError(t, err)
		assert.Equal(t, http.MethodPost, gotMethod)
		assert.Equal(t, "/chat/completions", gotPath)
		assert.Equal(t, "Bearer sk-test", gotAuth)

		var sent orRequest
		require.NoError(t, json.Unmarshal(gotBody, &sent))
		assert.Equal(t, "openai/gpt-4o", sent.Model)
		assert.Len(t, sent.Messages, 2)
		assert.Equal(t, "system", sent.Messages[0].Role)
		require.Len(t, sent.Tools, 1)
		assert.Equal(t, "get_weather", sent.Tools[0].Function.Name)
	})

	t.Run("omits the tools field when no tools are provided", func(t *testing.T) {
		// given
		var gotBody []byte
		o := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			gotBody, _ = io.ReadAll(r.Body)
			writeJSON(t, w, `{"choices":[{"message":{"content":"ok"}}],"usage":{}}`)
		})
		// when
		_, err := o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		require.NoError(t, err)
		assert.NotContains(t, string(gotBody), "tools")
	})

	t.Run("returns the assistant content and usage stats", func(t *testing.T) {
		// given
		o := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, `{
				"choices":[{"message":{"content":"Hello there"}}],
				"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}
			}`)
		})
		// when
		result, err := o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "Hello there", result.Content)
		assert.Equal(t, Stats{PromptTokens: 10, OutputTokens: 5, TotalTokens: 15}, result.Stats)
		assert.Empty(t, result.ToolCalls)
	})

	t.Run("parses tool calls from the response", func(t *testing.T) {
		// given
		o := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, `{
				"choices":[{"message":{"content":"","tool_calls":[
					{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Lisbon\"}"}}
				]}}],
				"usage":{}
			}`)
		})
		// when
		result, err := o.chat(t.Context(), []Message{UserMessage{Content: "weather?"}}, nil)
		// then
		require.NoError(t, err)
		require.Len(t, result.ToolCalls, 1)
		expected := ToolCall{ID: "call_1", Name: "get_weather", Arguments: map[string]any{"city": "Lisbon"}}
		assert.Equal(t, expected, result.ToolCalls[0])
	})

	t.Run("returns an error on a non-2xx status", func(t *testing.T) {
		// given: 400 is not retried (only 429 and 5xx are)
		o := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":{"message":"model not found"}}`)
		})
		// when
		result, err := o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "model not found")
	})

	t.Run("retries on 429 and then succeeds", func(t *testing.T) {
		// given: first call is rate-limited, second succeeds
		var calls atomic.Int32
		o := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1) == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			writeJSON(t, w, `{"choices":[{"message":{"content":"ok"}}],"usage":{}}`)
		})
		// when
		result, err := o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "ok", result.Content)
		assert.Equal(t, int32(2), calls.Load()) // the retry is the contract here
	})

	t.Run("returns an error when a 2xx response carries an error body", func(t *testing.T) {
		// given
		o := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, `{"error":{"message":"content filtered"}}`)
		})
		// when
		result, err := o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "content filtered")
	})

	t.Run("returns an error when the response body is malformed", func(t *testing.T) {
		// given
		o := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, `{"choices":`)
		})
		// when
		result, err := o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		assert.Nil(t, result)
		assert.Error(t, err)
	})

	t.Run("returns an error when the response contains no choices", func(t *testing.T) {
		// given
		o := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, `{"choices":[],"usage":{}}`)
		})
		// when
		result, err := o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "no choices")
	})
}

func TestModelInfo(t *testing.T) {
	t.Run("sends a GET request to the models endpoint with auth", func(t *testing.T) {
		// given
		var (
			gotMethod string
			gotPath   string
			gotAuth   string
		)
		o := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotPath = r.URL.Path
			gotAuth = r.Header.Get("Authorization")
			writeJSON(t, w, `{"data":[{"id":"openai/gpt-4o","name":"OpenAI: GPT-4o","context_length":128000}]}`)
		})
		// when
		_, err := o.modelInfo(t.Context())
		// then
		require.NoError(t, err)
		assert.Equal(t, http.MethodGet, gotMethod)
		assert.Equal(t, "/models", gotPath)
		assert.Equal(t, "Bearer sk-test", gotAuth)
	})

	t.Run("returns the name and context size of the configured model", func(t *testing.T) {
		// given
		o := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, `{"data":[
				{"id":"anthropic/claude-3","name":"Claude 3","context_length":200000},
				{"id":"openai/gpt-4o","name":"OpenAI: GPT-4o","context_length":128000}
			]}`)
		})
		// when
		result, err := o.modelInfo(t.Context())
		// then
		require.NoError(t, err)
		assert.Equal(t, ModelInfo{Name: "OpenAI: GPT-4o", ContextSize: 128000}, result)
	})

	t.Run("returns an error when the configured model is not listed", func(t *testing.T) {
		// given
		o := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, `{"data":[{"id":"anthropic/claude-3","name":"Claude 3","context_length":200000}]}`)
		})
		// when
		result, err := o.modelInfo(t.Context())
		// then
		assert.Equal(t, ModelInfo{}, result)
		assert.ErrorIs(t, err, ErrModelNotFound)
	})

	t.Run("returns an error on a non-2xx status", func(t *testing.T) {
		// given
		o := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":{"message":"invalid api key"}}`)
		})
		// when
		result, err := o.modelInfo(t.Context())
		// then
		assert.Equal(t, ModelInfo{}, result)
		assert.ErrorContains(t, err, "invalid api key")
	})

	t.Run("returns an error when the response body is malformed", func(t *testing.T) {
		// given
		o := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, `{"data":`)
		})
		// when
		result, err := o.modelInfo(t.Context())
		// then
		assert.Equal(t, ModelInfo{}, result)
		assert.Error(t, err)
	})
}

// --- test helpers ---

// newTestProvider spins up an httptest server running handler and returns an
// openrouter client pointed at it.
func newTestProvider(t testing.TB, handler http.HandlerFunc) *openrouter {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	o, err := newOpenRouter(Config{APIKey: "sk-test", Model: "openai/gpt-4o", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newOpenRouter: unexpected error: %v", err)
	}
	return o
}

func writeJSON(t testing.TB, w http.ResponseWriter, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if _, err := io.WriteString(w, body); err != nil {
		t.Fatalf("writing response body: %v", err)
	}
}

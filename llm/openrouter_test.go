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
		assert.Equal(t, openrouterBaseURL, result.config.BaseURL)
	})

	t.Run("keeps the configured base URL", func(t *testing.T) {
		// given
		baseURL := "https://proxy.example.com/api/v1"
		// when
		result, err := newOpenRouter(Config{APIKey: "sk-test", Model: "openai/gpt-4o", BaseURL: baseURL})
		// then
		require.NoError(t, err)
		assert.Equal(t, baseURL, result.config.BaseURL)
	})
}

func TestOpenRouterChat(t *testing.T) {
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
			writeSSE(t, w, `{"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}`, `[DONE]`)
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

		var sent orChatRequest
		require.NoError(t, json.Unmarshal(gotBody, &sent))
		assert.Equal(t, "openai/gpt-4o", sent.Model)
		assert.True(t, sent.Stream)
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
			writeSSE(t, w, `{"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}`, `[DONE]`)
		})
		// when
		_, err := o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		require.NoError(t, err)
		assert.NotContains(t, string(gotBody), "tools")
	})

	t.Run("includes max_tokens when configured", func(t *testing.T) {
		// given
		var gotBody []byte
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotBody, _ = io.ReadAll(r.Body)
			writeSSE(t, w, `{"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}`, `[DONE]`)
		}))
		t.Cleanup(server.Close)
		o, err := newOpenRouter(Config{APIKey: "sk-test", Model: "openai/gpt-4o", BaseURL: server.URL, MaxTokens: 256})
		require.NoError(t, err)
		// when
		_, err = o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		require.NoError(t, err)
		var sent orChatRequest
		require.NoError(t, json.Unmarshal(gotBody, &sent))
		assert.Equal(t, 256, sent.MaxTokens)
	})

	t.Run("omits max_tokens when not configured", func(t *testing.T) {
		// given
		var gotBody []byte
		o := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			gotBody, _ = io.ReadAll(r.Body)
			writeSSE(t, w, `{"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}`, `[DONE]`)
		})
		// when
		_, err := o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		require.NoError(t, err)
		assert.NotContains(t, string(gotBody), "max_tokens")
	})

	t.Run("does not set max_tokens when only effort is set", func(t *testing.T) {
		// given
		var gotBody []byte
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotBody, _ = io.ReadAll(r.Body)
			writeSSE(t, w, `{"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}`, `[DONE]`)
		}))
		t.Cleanup(server.Close)
		o, err := newOpenRouter(Config{APIKey: "sk-test", Model: "openai/gpt-4o", BaseURL: server.URL, Effort: EffortMax})
		require.NoError(t, err)
		// when
		_, err = o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		require.NoError(t, err)
		var sent orChatRequest
		require.NoError(t, json.Unmarshal(gotBody, &sent))
		assert.Zero(t, sent.MaxTokens)
		require.NotNil(t, sent.Reasoning)
		assert.Equal(t, "high", sent.Reasoning.Effort)
	})

	t.Run("joins streamed content and takes usage and finish reason from the stream", func(t *testing.T) {
		// given
		o := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			writeSSE(t, w,
				`{"choices":[{"delta":{"role":"assistant","content":"Hello"},"finish_reason":null}]}`,
				`{"choices":[{"delta":{"content":" there"},"finish_reason":"stop"}]}`,
				`{"choices":[{"delta":{"content":""},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
				`[DONE]`,
			)
		})
		// when
		result, err := o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "Hello there", result.Content)
		assert.Equal(t, "stop", result.StopReason)
		assert.Equal(t, Stats{PromptTokens: 10, OutputTokens: 5, TotalTokens: 15}, result.Stats)
		assert.Empty(t, result.ToolCalls)
	})

	t.Run("assembles tool calls from fragments by index", func(t *testing.T) {
		// given
		o := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			writeSSE(t, w,
				`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`,
				`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":"}}]}}]}`,
				`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"call_2","type":"function","function":{"name":"get_time","arguments":"{}"}}]}}]}`,
				`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"Lisbon\"}"}}]}}]}`,
				`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
				`[DONE]`,
			)
		})
		// when
		result, err := o.chat(t.Context(), []Message{UserMessage{Content: "weather?"}}, nil)
		// then
		require.NoError(t, err)
		expected := []ToolCall{
			{ID: "call_1", Name: "get_weather", Arguments: map[string]any{"city": "Lisbon"}},
			{ID: "call_2", Name: "get_time", Arguments: map[string]any{}},
		}
		assert.Equal(t, expected, result.ToolCalls)
		assert.Equal(t, "tool_calls", result.StopReason)
	})

	t.Run("returns an error when a tool call index skips ahead", func(t *testing.T) {
		// given
		o := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			writeSSE(t, w,
				`{"choices":[{"delta":{"tool_calls":[{"index":3,"id":"call_1","function":{"name":"get_weather","arguments":"{}"}}]}}]}`,
				`[DONE]`,
			)
		})
		// when
		result, err := o.chat(t.Context(), []Message{UserMessage{Content: "weather?"}}, nil)
		// then
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "tool call index")
	})

	t.Run("ignores keep-alive comments", func(t *testing.T) {
		// given
		o := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, ": OPENROUTER PROCESSING\n\n: OPENROUTER PROCESSING\n\n")
			writeSSE(t, w, `{"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}`, `[DONE]`)
		})
		// when
		result, err := o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		require.NoError(t, err)
		assert.Equal(t, "ok", result.Content)
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
			writeSSE(t, w, `{"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}]}`, `[DONE]`)
		})
		// when
		result, err := o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "ok", result.Content)
		assert.Equal(t, int32(2), calls.Load())
	})

	t.Run("returns an error when a chunk mid-stream carries an error", func(t *testing.T) {
		// given
		o := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			writeSSE(t, w,
				`{"choices":[{"delta":{"content":"Hel"}}]}`,
				`{"error":{"code":"server_error","message":"Provider disconnected"},"choices":[{"delta":{"content":""},"finish_reason":"error"}]}`,
			)
		})
		// when
		result, err := o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "Provider disconnected")
	})

	t.Run("returns an error when a chunk is not valid JSON", func(t *testing.T) {
		// given
		o := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			writeSSE(t, w, `{"choices":`, `[DONE]`)
		})
		// when
		result, err := o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "openrouter: reading stream")
	})

	t.Run("returns an error when the stream ends before done", func(t *testing.T) {
		// given
		o := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			writeSSE(t, w, `{"choices":[{"delta":{"content":"Hel"}}]}`)
		})
		// when
		result, err := o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "stream ended before done")
	})

	t.Run("returns an error when the response contains no choices", func(t *testing.T) {
		// given
		o := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			writeSSE(t, w, `{"choices":[],"usage":{}}`, `[DONE]`)
		})
		// when
		result, err := o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "no choices")
	})
}

func TestOpenRouterModelInfo(t *testing.T) {
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
		assert.Equal(t, &ModelInfo{Name: "openai/gpt-4o", ContextSize: 128000}, result)
	})

	t.Run("returns an error when the configured model is not listed", func(t *testing.T) {
		// given
		o := newTestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, `{"data":[{"id":"anthropic/claude-3","name":"Claude 3","context_length":200000}]}`)
		})
		// when
		result, err := o.modelInfo(t.Context())
		// then
		assert.Nil(t, result)
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
		assert.Nil(t, result)
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
		assert.Nil(t, result)
		assert.Error(t, err)
	})
}

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

func writeSSE(t testing.TB, w http.ResponseWriter, events ...string) {
	t.Helper()
	w.Header().Set("Content-Type", "text/event-stream")
	for _, event := range events {
		if _, err := io.WriteString(w, "data: "+event+"\n\n"); err != nil {
			t.Fatalf("writing response body: %v", err)
		}
	}
}

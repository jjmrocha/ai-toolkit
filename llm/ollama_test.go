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

func TestNewOllama(t *testing.T) {
	t.Run("does not require an API key", func(t *testing.T) {
		// when
		result, err := newOllama(Config{Model: "llama3.2"})
		// then
		require.NoError(t, err)
		require.NotNil(t, result)
	})

	t.Run("applies the default base URL", func(t *testing.T) {
		// when
		result, err := newOllama(Config{Model: "llama3.2"})
		// then
		require.NoError(t, err)
		assert.Equal(t, ollamaBaseURL, result.cfg.BaseURL)
	})
}

func TestOllamaChat(t *testing.T) {
	t.Run("posts to /api/chat with stream disabled and no auth by default", func(t *testing.T) {
		// given
		var (
			gotMethod string
			gotPath   string
			gotAuth   string
			gotBody   []byte
		)
		o := newTestOllama(t, func(w http.ResponseWriter, r *http.Request) {
			gotMethod, gotPath, gotAuth = r.Method, r.URL.Path, r.Header.Get("Authorization")
			gotBody, _ = io.ReadAll(r.Body)
			writeJSON(t, w, `{"message":{"role":"assistant","content":"ok"},"done":true}`)
		})
		// when
		_, err := o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		require.NoError(t, err)
		assert.Equal(t, http.MethodPost, gotMethod)
		assert.Equal(t, "/api/chat", gotPath)
		assert.Empty(t, gotAuth)

		var sent ollamaChatRequest
		require.NoError(t, json.Unmarshal(gotBody, &sent))
		assert.Equal(t, "llama3.2", sent.Model)
		assert.False(t, sent.Stream)
		assert.Len(t, sent.Messages, 1)
	})

	t.Run("includes num_predict when max tokens is configured", func(t *testing.T) {
		// given
		var gotBody []byte
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotBody, _ = io.ReadAll(r.Body)
			writeJSON(t, w, `{"message":{"role":"assistant","content":"ok"},"done":true}`)
		}))
		t.Cleanup(server.Close)
		o, err := newOllama(Config{Model: "llama3.2", BaseURL: server.URL, MaxTokens: 256})
		require.NoError(t, err)
		// when
		_, err = o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		require.NoError(t, err)
		var sent ollamaChatRequest
		require.NoError(t, json.Unmarshal(gotBody, &sent))
		require.NotNil(t, sent.Options)
		assert.Equal(t, 256, sent.Options.NumPredict)
	})

	t.Run("omits options when max tokens is not configured", func(t *testing.T) {
		// given
		var gotBody []byte
		o := newTestOllama(t, func(w http.ResponseWriter, r *http.Request) {
			gotBody, _ = io.ReadAll(r.Body)
			writeJSON(t, w, `{"message":{"role":"assistant","content":"ok"},"done":true}`)
		})
		// when
		_, err := o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		require.NoError(t, err)
		assert.NotContains(t, string(gotBody), "options")
	})

	t.Run("returns content and summed token stats", func(t *testing.T) {
		// given
		o := newTestOllama(t, func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, `{
				"message":{"role":"assistant","content":"Hello there"},
				"done":true,"prompt_eval_count":10,"eval_count":5
			}`)
		})
		// when
		result, err := o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		require.NoError(t, err)
		assert.Equal(t, "Hello there", result.Content)
		assert.Equal(t, Stats{PromptTokens: 10, OutputTokens: 5, TotalTokens: 15}, result.Stats)
	})

	t.Run("parses tool calls with object arguments", func(t *testing.T) {
		// given
		o := newTestOllama(t, func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, `{
				"message":{"role":"assistant","content":"","tool_calls":[
					{"function":{"name":"get_weather","arguments":{"city":"Tokyo"}}}
				]},
				"done":true
			}`)
		})
		// when
		result, err := o.chat(t.Context(), []Message{UserMessage{Content: "weather?"}}, nil)
		// then
		require.NoError(t, err)
		require.Len(t, result.ToolCalls, 1)
		assert.Equal(t, ToolCall{Name: "get_weather", Arguments: map[string]any{"city": "Tokyo"}}, result.ToolCalls[0])
	})

	t.Run("returns an error when the body carries an error field", func(t *testing.T) {
		// given
		o := newTestOllama(t, func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, `{"error":"model is required"}`)
		})
		// when
		result, err := o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "model is required")
	})

	t.Run("returns an error on a non-2xx status", func(t *testing.T) {
		// given: 400 is not retried
		o := newTestOllama(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"bad request"}`)
		})
		// when
		result, err := o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "bad request")
	})

	t.Run("retries on 429 and then succeeds", func(t *testing.T) {
		// given
		var calls atomic.Int32
		o := newTestOllama(t, func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1) == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			writeJSON(t, w, `{"message":{"role":"assistant","content":"ok"},"done":true}`)
		})
		// when
		result, err := o.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		require.NoError(t, err)
		assert.Equal(t, "ok", result.Content)
		assert.Equal(t, int32(2), calls.Load())
	})

}

func TestOllamaModelInfo(t *testing.T) {
	t.Run("posts to /api/show and returns the context size", func(t *testing.T) {
		// given
		var gotPath string
		var gotBody []byte
		o := newTestOllama(t, func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotBody, _ = io.ReadAll(r.Body)
			writeJSON(t, w, `{"model_info":{"general.architecture":"llama","llama.context_length":8192}}`)
		})
		// when
		result, err := o.modelInfo(t.Context())
		// then
		require.NoError(t, err)
		assert.Equal(t, "/api/show", gotPath)
		assert.Equal(t, &ModelInfo{Name: "llama3.2", ContextSize: 8192}, result)

		var sent ollamaShowRequest
		require.NoError(t, json.Unmarshal(gotBody, &sent))
		assert.Equal(t, "llama3.2", sent.Model)
	})

	t.Run("returns ErrModelNotFound on a 404", func(t *testing.T) {
		// given
		o := newTestOllama(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":"model not found"}`)
		})
		// when
		result, err := o.modelInfo(t.Context())
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrModelNotFound)
	})

	t.Run("returns an error on a non-2xx status other than 404", func(t *testing.T) {
		// given: 400 is not retried
		o := newTestOllama(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"bad request"}`)
		})
		// when
		result, err := o.modelInfo(t.Context())
		// then
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "bad request")
	})

	t.Run("returns an error when the body carries an error field", func(t *testing.T) {
		// given
		o := newTestOllama(t, func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, `{"error":"something went wrong"}`)
		})
		// when
		result, err := o.modelInfo(t.Context())
		// then
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "something went wrong")
	})
}

func newTestOllama(t testing.TB, handler http.HandlerFunc) *ollama {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	o, err := newOllama(Config{Model: "llama3.2", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newOllama: unexpected error: %v", err)
	}
	return o
}

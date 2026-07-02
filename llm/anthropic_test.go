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

func TestNewAnthropic(t *testing.T) {
	t.Run("returns error when API key is missing", func(t *testing.T) {
		// when
		_, err := newAnthropic(Config{Model: "claude-opus-4-8"})
		// then
		assert.ErrorIs(t, err, ErrMissingAPIKey)
	})

	t.Run("applies the default base URL when none is provided", func(t *testing.T) {
		// when
		result, err := newAnthropic(Config{APIKey: "sk-test", Model: "claude-opus-4-8"})
		// then
		require.NoError(t, err)
		assert.Equal(t, anthropicBaseURL, result.cfg.BaseURL)
	})

	t.Run("keeps the configured base URL", func(t *testing.T) {
		// given
		baseURL := "https://proxy.example.com/v1"
		// when
		result, err := newAnthropic(Config{APIKey: "sk-test", Model: "claude-opus-4-8", BaseURL: baseURL})
		// then
		require.NoError(t, err)
		assert.Equal(t, baseURL, result.cfg.BaseURL)
	})

	t.Run("applies the default max tokens when none is provided", func(t *testing.T) {
		// when
		result, err := newAnthropic(Config{APIKey: "sk-test", Model: "claude-opus-4-8"})
		// then
		require.NoError(t, err)
		assert.Equal(t, defaultMaxTokens, result.cfg.MaxTokens)
	})

	t.Run("keeps the configured max tokens", func(t *testing.T) {
		// when
		result, err := newAnthropic(Config{APIKey: "sk-test", Model: "claude-opus-4-8", MaxTokens: 1024})
		// then
		require.NoError(t, err)
		assert.Equal(t, 1024, result.cfg.MaxTokens)
	})
}

func TestAnthropicChat(t *testing.T) {
	t.Run("sends a POST carrying auth headers, model, system, max tokens, messages and tools", func(t *testing.T) {
		// given
		var (
			gotMethod  string
			gotPath    string
			gotAPIKey  string
			gotVersion string
			gotBody    []byte
		)
		a := newTestAnthropic(t, func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotPath = r.URL.Path
			gotAPIKey = r.Header.Get("x-api-key")
			gotVersion = r.Header.Get("anthropic-version")
			gotBody, _ = io.ReadAll(r.Body)
			writeJSON(t, w, `{"content":[{"type":"text","text":"ok"}],"usage":{}}`)
		})
		messages := []Message{SystemMessage{Content: "Be brief"}, UserMessage{Content: "Hi"}}
		tools := []Tool{{Name: "get_weather", Description: "Get the weather", Schema: map[string]any{"type": "object"}}}
		// when
		_, err := a.chat(t.Context(), messages, tools)
		// then
		require.NoError(t, err)
		assert.Equal(t, http.MethodPost, gotMethod)
		assert.Equal(t, "/messages", gotPath)
		assert.Equal(t, "sk-test", gotAPIKey)
		assert.Equal(t, anthropicVersion, gotVersion)

		var sent anthropicChatRequest
		require.NoError(t, json.Unmarshal(gotBody, &sent))
		assert.Equal(t, "claude-opus-4-8", sent.Model)
		assert.Equal(t, defaultMaxTokens, sent.MaxTokens)
		assert.Equal(t, "Be brief", sent.System)
		require.Len(t, sent.Messages, 1)
		assert.Equal(t, "user", sent.Messages[0].Role)
		require.Len(t, sent.Tools, 1)
		assert.Equal(t, "get_weather", sent.Tools[0].Name)
	})

	t.Run("omits the tools field when no tools are provided", func(t *testing.T) {
		// given
		var gotBody []byte
		a := newTestAnthropic(t, func(w http.ResponseWriter, r *http.Request) {
			gotBody, _ = io.ReadAll(r.Body)
			writeJSON(t, w, `{"content":[{"type":"text","text":"ok"}],"usage":{}}`)
		})
		// when
		_, err := a.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		require.NoError(t, err)
		assert.NotContains(t, string(gotBody), "tools")
	})

	t.Run("returns the assistant content and usage stats", func(t *testing.T) {
		// given
		a := newTestAnthropic(t, func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, `{
				"content":[{"type":"text","text":"Hello there"}],
				"usage":{"input_tokens":10,"output_tokens":5}
			}`)
		})
		// when
		result, err := a.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "Hello there", result.Content)
		assert.Equal(t, Stats{PromptTokens: 10, OutputTokens: 5, TotalTokens: 15}, result.Stats)
		assert.Empty(t, result.ToolCalls)
	})

	t.Run("parses tool calls from the response", func(t *testing.T) {
		// given
		a := newTestAnthropic(t, func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, `{
				"content":[{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"Lisbon"}}],
				"usage":{}
			}`)
		})
		// when
		result, err := a.chat(t.Context(), []Message{UserMessage{Content: "weather?"}}, nil)
		// then
		require.NoError(t, err)
		require.Len(t, result.ToolCalls, 1)
		expected := ToolCall{ID: "toolu_1", Name: "get_weather", Arguments: map[string]any{"city": "Lisbon"}}
		assert.Equal(t, expected, result.ToolCalls[0])
	})

	t.Run("returns an error on a non-2xx status", func(t *testing.T) {
		// given: 400 is not retried (only 429 and 5xx are)
		a := newTestAnthropic(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"model not found"}}`)
		})
		// when
		result, err := a.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "model not found")
	})

	t.Run("retries on 429 and then succeeds", func(t *testing.T) {
		// given: first call is rate-limited, second succeeds
		var calls atomic.Int32
		a := newTestAnthropic(t, func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1) == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			writeJSON(t, w, `{"content":[{"type":"text","text":"ok"}],"usage":{}}`)
		})
		// when
		result, err := a.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "ok", result.Content)
		assert.Equal(t, int32(2), calls.Load()) // the retry is the contract here
	})

	t.Run("returns an error when the response body is malformed", func(t *testing.T) {
		// given
		a := newTestAnthropic(t, func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, `{"content":`)
		})
		// when
		result, err := a.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		assert.Nil(t, result)
		assert.Error(t, err)
	})
}

func TestAnthropicModelInfo(t *testing.T) {
	t.Run("sends a GET request to the model endpoint with auth headers", func(t *testing.T) {
		// given
		var (
			gotMethod  string
			gotPath    string
			gotAPIKey  string
			gotVersion string
		)
		a := newTestAnthropic(t, func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotPath = r.URL.Path
			gotAPIKey = r.Header.Get("x-api-key")
			gotVersion = r.Header.Get("anthropic-version")
			writeJSON(t, w, `{"id":"claude-opus-4-8","display_name":"Claude Opus 4.8","max_input_tokens":1000000}`)
		})
		// when
		_, err := a.modelInfo(t.Context())
		// then
		require.NoError(t, err)
		assert.Equal(t, http.MethodGet, gotMethod)
		assert.Equal(t, "/models/claude-opus-4-8", gotPath)
		assert.Equal(t, "sk-test", gotAPIKey)
		assert.Equal(t, anthropicVersion, gotVersion)
	})

	t.Run("returns the name and context size of the configured model", func(t *testing.T) {
		// given
		a := newTestAnthropic(t, func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, `{"id":"claude-opus-4-8","display_name":"Claude Opus 4.8","max_input_tokens":1000000}`)
		})
		// when
		result, err := a.modelInfo(t.Context())
		// then
		require.NoError(t, err)
		assert.Equal(t, &ModelInfo{Name: "Claude Opus 4.8", ContextSize: 1000000}, result)
	})

	t.Run("returns ErrModelNotFound when the model is not found", func(t *testing.T) {
		// given
		a := newTestAnthropic(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"not_found_error","message":"model not found"}}`)
		})
		// when
		result, err := a.modelInfo(t.Context())
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrModelNotFound)
	})

	t.Run("returns an error on a non-2xx status", func(t *testing.T) {
		// given
		a := newTestAnthropic(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"invalid api key"}}`)
		})
		// when
		result, err := a.modelInfo(t.Context())
		// then
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "invalid api key")
	})

	t.Run("returns an error when the response body is malformed", func(t *testing.T) {
		// given
		a := newTestAnthropic(t, func(w http.ResponseWriter, r *http.Request) {
			writeJSON(t, w, `{"id":`)
		})
		// when
		result, err := a.modelInfo(t.Context())
		// then
		assert.Nil(t, result)
		assert.Error(t, err)
	})
}

// --- test helpers ---

// newTestAnthropic spins up an httptest server running handler and returns an
// anthropic client pointed at it.
func newTestAnthropic(t testing.TB, handler http.HandlerFunc) *anthropic {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	a, err := newAnthropic(Config{APIKey: "sk-test", Model: "claude-opus-4-8", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newAnthropic: unexpected error: %v", err)
	}
	return a
}

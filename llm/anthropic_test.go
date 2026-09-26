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
		assert.Equal(t, anthropicBaseURL, result.config.BaseURL)
	})

	t.Run("keeps the configured base URL", func(t *testing.T) {
		// given
		baseURL := "https://proxy.example.com/v1"
		// when
		result, err := newAnthropic(Config{APIKey: "sk-test", Model: "claude-opus-4-8", BaseURL: baseURL})
		// then
		require.NoError(t, err)
		assert.Equal(t, baseURL, result.config.BaseURL)
	})

	t.Run("applies the default max tokens when none is provided", func(t *testing.T) {
		// when
		result, err := newAnthropic(Config{APIKey: "sk-test", Model: "claude-opus-4-8"})
		// then
		require.NoError(t, err)
		assert.Equal(t, defaultMaxTokens, result.config.MaxTokens)
	})

	t.Run("keeps the configured max tokens", func(t *testing.T) {
		// when
		result, err := newAnthropic(Config{APIKey: "sk-test", Model: "claude-opus-4-8", MaxTokens: 1024})
		// then
		require.NoError(t, err)
		assert.Equal(t, 1024, result.config.MaxTokens)
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
			writeSSE(t, w, anthropicTextStream("ok")...)
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
		assert.True(t, sent.Stream)
		assert.Equal(t, defaultMaxTokens, sent.MaxTokens)
		require.Len(t, sent.System, 1)
		assert.Equal(t, "Be brief", sent.System[0].Text)
		require.Len(t, sent.Messages, 1)
		assert.Equal(t, "user", sent.Messages[0].Role)
		require.Len(t, sent.Tools, 1)
		assert.Equal(t, "get_weather", sent.Tools[0].Name)
		assert.Equal(t, &anthropicThinking{Type: "adaptive"}, sent.Thinking)
		assert.Equal(t, &anthropicOutputConfig{Effort: "low"}, sent.OutputConfig)
	})

	t.Run("omits the tools field when no tools are provided", func(t *testing.T) {
		// given
		var gotBody []byte
		a := newTestAnthropic(t, func(w http.ResponseWriter, r *http.Request) {
			gotBody, _ = io.ReadAll(r.Body)
			writeSSE(t, w, anthropicTextStream("ok")...)
		})
		// when
		_, err := a.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		require.NoError(t, err)
		assert.NotContains(t, string(gotBody), "tools")
	})

	t.Run("joins streamed text and takes usage from the start and end of the message", func(t *testing.T) {
		// given
		a := newTestAnthropic(t, func(w http.ResponseWriter, r *http.Request) {
			writeSSE(t, w,
				`{"type":"message_start","message":{"content":[],"stop_reason":null,"usage":{"input_tokens":10,"cache_creation_input_tokens":3,"cache_read_input_tokens":2,"output_tokens":1}}}`,
				`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
				`{"type":"ping"}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":" there"}}`,
				`{"type":"content_block_stop","index":0}`,
				`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":5}}`,
				`{"type":"message_stop"}`,
			)
		})
		// when
		result, err := a.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "Hello there", result.Content)
		assert.Equal(t, "end_turn", result.StopReason)
		expected := Stats{PromptTokens: 15, OutputTokens: 5, TotalTokens: 20, CacheWriteTokens: 3, CacheReadTokens: 2}
		assert.Equal(t, expected, result.Stats)
		assert.Empty(t, result.ToolCalls)
	})

	t.Run("assembles tool use input from partial JSON", func(t *testing.T) {
		// given
		a := newTestAnthropic(t, func(w http.ResponseWriter, r *http.Request) {
			writeSSE(t, w,
				`{"type":"message_start","message":{"content":[],"usage":{}}}`,
				`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{}}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"Lisbon\"}"}}`,
				`{"type":"content_block_stop","index":0}`,
				`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_2","name":"get_time","input":{}}}`,
				`{"type":"content_block_stop","index":1}`,
				`{"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":9}}`,
				`{"type":"message_stop"}`,
			)
		})
		// when
		result, err := a.chat(t.Context(), []Message{UserMessage{Content: "weather?"}}, nil)
		// then
		require.NoError(t, err)
		expected := []ToolCall{
			{ID: "toolu_1", Name: "get_weather", Arguments: map[string]any{"city": "Lisbon"}},
			{ID: "toolu_2", Name: "get_time", Arguments: map[string]any{}},
		}
		assert.Equal(t, expected, result.ToolCalls)
	})

	t.Run("keeps thinking blocks with their signature for the next turn", func(t *testing.T) {
		// given
		a := newTestAnthropic(t, func(w http.ResponseWriter, r *http.Request) {
			writeSSE(t, w,
				`{"type":"message_start","message":{"content":[],"usage":{}}}`,
				`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Let me "}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"think."}}`,
				`{"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-123"}}`,
				`{"type":"content_block_stop","index":0}`,
				`{"type":"content_block_start","index":1,"content_block":{"type":"redacted_thinking","data":"opaque"}}`,
				`{"type":"content_block_stop","index":1}`,
				`{"type":"content_block_start","index":2,"content_block":{"type":"text","text":""}}`,
				`{"type":"content_block_delta","index":2,"delta":{"type":"text_delta","text":"Done"}}`,
				`{"type":"content_block_stop","index":2}`,
				`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{}}`,
				`{"type":"message_stop"}`,
			)
		})
		// when
		result, err := a.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		require.NoError(t, err)
		assert.Equal(t, "Done", result.Content)
		expected := []anthropicContentBlock{
			{Type: "thinking", Thinking: "Let me think.", Signature: "sig-123"},
			{Type: "redacted_thinking", Data: "opaque"},
			{Type: "text", Text: "Done"},
		}
		assert.Equal(t, expected, result.raw)
	})

	t.Run("returns an error when a delta names a block that was not started", func(t *testing.T) {
		// given
		a := newTestAnthropic(t, func(w http.ResponseWriter, r *http.Request) {
			writeSSE(t, w,
				`{"type":"message_start","message":{"content":[],"usage":{}}}`,
				`{"type":"content_block_delta","index":2,"delta":{"type":"text_delta","text":"Hello"}}`,
				`{"type":"message_stop"}`,
			)
		})
		// when
		result, err := a.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "content block index")
	})

	t.Run("returns an error when the stream carries an error event", func(t *testing.T) {
		// given
		a := newTestAnthropic(t, func(w http.ResponseWriter, r *http.Request) {
			writeSSE(t, w,
				`{"type":"message_start","message":{"content":[],"usage":{}}}`,
				`{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`,
			)
		})
		// when
		result, err := a.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "Overloaded")
	})

	t.Run("returns an error when the stream ends before message_stop", func(t *testing.T) {
		// given
		a := newTestAnthropic(t, func(w http.ResponseWriter, r *http.Request) {
			writeSSE(t, w,
				`{"type":"message_start","message":{"content":[],"usage":{}}}`,
				`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			)
		})
		// when
		result, err := a.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "stream ended before done")
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
			writeSSE(t, w, anthropicTextStream("ok")...)
		})
		// when
		result, err := a.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "ok", result.Content)
		assert.Equal(t, int32(2), calls.Load())
	})

	t.Run("returns an error when an event is not valid JSON", func(t *testing.T) {
		// given
		a := newTestAnthropic(t, func(w http.ResponseWriter, r *http.Request) {
			writeSSE(t, w, `{"type":`)
		})
		// when
		result, err := a.chat(t.Context(), []Message{UserMessage{Content: "Hi"}}, nil)
		// then
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "anthropic: reading stream")
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
		assert.Equal(t, &ModelInfo{Name: "claude-opus-4-8", ContextSize: 1000000}, result)
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

func newTestAnthropic(t testing.TB, handler http.HandlerFunc) *anthropic {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	a, err := newAnthropic(Config{APIKey: "sk-test", Model: "claude-opus-4-8", BaseURL: server.URL, Effort: EffortOff})
	if err != nil {
		t.Fatalf("newAnthropic: unexpected error: %v", err)
	}
	return a
}

func anthropicTextStream(text string) []string {
	return []string{
		`{"type":"message_start","message":{"content":[],"usage":{}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"` + text + `"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{}}`,
		`{"type":"message_stop"}`,
	}
}

package decision

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewOllama(t *testing.T) {
	t.Run("succeeds without an API key", func(t *testing.T) {
		// when
		result, err := newOllama(Config{Model: "nimble"})
		// then
		require.NoError(t, err)
		assert.Equal(t, "nimble", result.currentModel())
	})

	t.Run("applies the default base URL when none is provided", func(t *testing.T) {
		// when
		result, err := newOllama(Config{Model: "nimble"})
		// then
		require.NoError(t, err)
		assert.Equal(t, ollamaBaseURL, result.config.BaseURL)
	})
}

func TestOllamaAsk(t *testing.T) {
	t.Run("sends a POST request carrying model, state and questions", func(t *testing.T) {
		// given
		var (
			gotMethod string
			gotPath   string
			gotBody   []byte
		)
		o := newTestOllama(t, func(w http.ResponseWriter, r *http.Request) {
			gotMethod = r.Method
			gotPath = r.URL.Path
			gotBody, _ = io.ReadAll(r.Body)
			writeJSON(t, w, `{"model":"nimble","answers":{"refund":{"type":"noul","noul":0.997}},"usage":{"input_tokens":841,"output_tokens":3}}`)
		})
		request := Request{
			State:     "I was charged twice. Please refund the extra payment.",
			Questions: map[string]Question{"refund": YesNo{Instructions: "Does the customer explicitly ask for a refund?"}},
		}
		expectedBody := map[string]any{
			"model": "nimble",
			"state": "I was charged twice. Please refund the extra payment.",
			"questions": map[string]any{
				"refund": map[string]any{
					"type":         "noul",
					"instructions": "Does the customer explicitly ask for a refund?",
				},
			},
		}
		// when
		_, err := o.ask(t.Context(), request)
		// then
		require.NoError(t, err)
		assert.Equal(t, http.MethodPost, gotMethod)
		assert.Equal(t, "/v1/systemone", gotPath)

		var body map[string]any
		require.NoError(t, json.Unmarshal(gotBody, &body))
		assert.Equal(t, expectedBody, body)
	})

	t.Run("sends no authorization header even when an API key is set", func(t *testing.T) {
		// given
		gotAuth := "not called"
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			writeJSON(t, w, `{"answers":{"refund":{"type":"noul","noul":0.997}},"usage":{}}`)
		}))
		t.Cleanup(server.Close)
		o, err := newOllama(Config{Model: "nimble", BaseURL: server.URL, APIKey: "sk-test"})
		require.NoError(t, err)
		request := Request{Questions: map[string]Question{"refund": YesNo{Instructions: "Refund?"}}}
		// when
		_, err = o.ask(t.Context(), request)
		// then
		require.NoError(t, err)
		assert.Empty(t, gotAuth)
	})

	t.Run("returns the decoded answers and usage", func(t *testing.T) {
		// given
		o := newTestOllama(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(t, w, `{"model":"nimble","answers":{"refund":{"type":"noul","noul":0.997}},"usage":{"input_tokens":841,"output_tokens":3}}`)
		})
		request := Request{
			State:     "I was charged twice.",
			Questions: map[string]Question{"refund": YesNo{Instructions: "Refund?"}},
		}
		expected := &Response{
			Answers: map[string]Answer{"refund": YesNoAnswer{Value: 0.997}},
			Stats:   Stats{PromptTokens: 841},
		}
		// when
		result, err := o.ask(t.Context(), request)
		// then
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("asks a choice, a yes/no and a score question in one request", func(t *testing.T) {
		// given
		var gotBody []byte
		o := newTestOllama(t, func(w http.ResponseWriter, r *http.Request) {
			gotBody, _ = io.ReadAll(r.Body)
			writeJSON(t, w, `{
				"model": "nimble",
				"answers": {
					"team": {"type": "choice", "choice": "billing", "probabilities": {"billing": 0.984, "technical": 0.011, "other": 0.005}, "confidence": 0.918},
					"refund": {"type": "noul", "noul": 0.997},
					"urgency": {"type": "score", "score": 0.693, "legend": {"0": "Routine", "1": "Soon", "2": "Urgent"}, "probabilities": {"0": 0.482, "1": 0.342, "2": 0.175}, "confidence": 0.068}
				},
				"usage": {"input_tokens": 841, "output_tokens": 3}
			}`)
		})
		request := Request{
			State: "I was charged twice. Please refund the extra payment.",
			Questions: map[string]Question{
				"team": Choice{
					Instructions: "Which team should handle this ticket?",
					Options:      map[string]string{"billing": "Payments and refunds", "technical": "Bugs and integrations", "other": "None of the above"},
				},
				"refund":  YesNo{Instructions: "Does the customer explicitly ask for a refund?"},
				"urgency": Score{Instructions: "How urgent is this ticket?", Levels: []string{"Routine", "Soon", "Urgent"}},
			},
		}
		expectedBody := map[string]any{
			"model": "nimble",
			"state": "I was charged twice. Please refund the extra payment.",
			"questions": map[string]any{
				"team": map[string]any{
					"type":         "choice",
					"instructions": "Which team should handle this ticket?",
					"criteria":     map[string]any{"billing": "Payments and refunds", "technical": "Bugs and integrations", "other": "None of the above"},
				},
				"refund": map[string]any{
					"type":         "noul",
					"instructions": "Does the customer explicitly ask for a refund?",
				},
				"urgency": map[string]any{
					"type":         "score",
					"instructions": "How urgent is this ticket?",
					"criteria":     []any{"Routine", "Soon", "Urgent"},
				},
			},
		}
		expected := &Response{
			Answers: map[string]Answer{
				"team": ChoiceAnswer{
					Selected:      "billing",
					Probabilities: map[string]float64{"billing": 0.984, "technical": 0.011, "other": 0.005},
					Confidence:    0.918,
				},
				"refund": YesNoAnswer{Value: 0.997},
				"urgency": ScoreAnswer{
					Score:         0.693,
					Legend:        []string{"Routine", "Soon", "Urgent"},
					Probabilities: []float64{0.482, 0.342, 0.175},
					Confidence:    0.068,
				},
			},
			Stats: Stats{PromptTokens: 841},
		}
		// when
		result, err := o.ask(t.Context(), request)
		// then
		require.NoError(t, err)
		assert.Equal(t, expected, result)

		var body map[string]any
		require.NoError(t, json.Unmarshal(gotBody, &body))
		assert.Equal(t, expectedBody, body)
	})

	t.Run("returns an error for an unexpected status", func(t *testing.T) {
		// given
		o := newTestOllama(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			writeJSON(t, w, `{"error":"model 'nimble' not found"}`)
		})
		request := Request{Questions: map[string]Question{"refund": YesNo{Instructions: "Refund?"}}}
		// when
		result, err := o.ask(t.Context(), request)
		// then
		assert.Nil(t, result)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "404")
		assert.Contains(t, err.Error(), "model 'nimble' not found")
	})

	t.Run("returns ErrMissingAnswer when a question goes unanswered", func(t *testing.T) {
		// given
		o := newTestOllama(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(t, w, `{"model":"nimble","answers":{},"usage":{}}`)
		})
		request := Request{Questions: map[string]Question{"refund": YesNo{Instructions: "Refund?"}}}
		// when
		result, err := o.ask(t.Context(), request)
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrMissingAnswer)
	})
}

func newTestOllama(t testing.TB, handler http.HandlerFunc) *ollama {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	o, err := newOllama(Config{Model: "nimble", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newOllama: unexpected error: %v", err)
	}
	return o
}

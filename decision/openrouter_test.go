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

func TestNewOpenRouter(t *testing.T) {
	t.Run("returns error when API key is missing", func(t *testing.T) {
		// when
		_, err := newOpenRouter(Config{Model: "typesafe/jev-1.13"})
		// then
		assert.ErrorIs(t, err, ErrMissingAPIKey)
	})

	t.Run("applies the default base URL when none is provided", func(t *testing.T) {
		// when
		result, err := newOpenRouter(Config{APIKey: "sk-test", Model: "typesafe/jev-1.13"})
		// then
		require.NoError(t, err)
		assert.Equal(t, openrouterBaseURL, result.config.BaseURL)
	})

	t.Run("keeps the configured base URL", func(t *testing.T) {
		// given
		baseURL := "https://proxy.example.com/api/alpha"
		// when
		result, err := newOpenRouter(Config{APIKey: "sk-test", Model: "typesafe/jev-1.13", BaseURL: baseURL})
		// then
		require.NoError(t, err)
		assert.Equal(t, baseURL, result.config.BaseURL)
	})
}

func TestOpenRouterAsk(t *testing.T) {
	t.Run("sends a POST request carrying auth, model, state and questions", func(t *testing.T) {
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
			writeJSON(t, w, `{"model":"typesafe/jev-1.13-20260917","answers":{"is_urgent":{"type":"noul","noul":0.95}},"usage":{"input_tokens":296,"output_tokens":20,"cost":0.0000124}}`)
		})
		request := Request{
			State:     "Help! My payouts have been failing for 3 days.",
			Questions: map[string]Question{"is_urgent": Noul{Instructions: "Does this convey urgency?"}},
		}
		expectedBody := map[string]any{
			"model": "typesafe/jev-1.13",
			"state": "Help! My payouts have been failing for 3 days.",
			"questions": map[string]any{
				"is_urgent": map[string]any{
					"type":         "noul",
					"instructions": "Does this convey urgency?",
				},
			},
		}
		// when
		_, err := o.ask(t.Context(), request)
		// then
		require.NoError(t, err)
		assert.Equal(t, http.MethodPost, gotMethod)
		assert.Equal(t, "/decisions", gotPath)
		assert.Equal(t, "Bearer sk-test", gotAuth)

		var body map[string]any
		require.NoError(t, json.Unmarshal(gotBody, &body))
		assert.Equal(t, expectedBody, body)
	})

	t.Run("returns the decoded answers and usage", func(t *testing.T) {
		// given
		o := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(t, w, `{"model":"typesafe/jev-1.13-20260917","answers":{"is_urgent":{"type":"noul","noul":0.95}},"usage":{"input_tokens":296,"output_tokens":20,"cost":0.0000124}}`)
		})
		request := Request{
			State:     "Help!",
			Questions: map[string]Question{"is_urgent": Noul{Instructions: "Does this convey urgency?"}},
		}
		expected := &Response{
			Model:   "typesafe/jev-1.13-20260917",
			Answers: map[string]Answer{"is_urgent": NoulAnswer{Value: 0.95}},
			Stats:   Stats{InputTokens: 296},
		}
		// when
		result, err := o.ask(t.Context(), request)
		// then
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("returns an error for an unexpected status", func(t *testing.T) {
		// given
		o := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			writeJSON(t, w, `{"error":{"message":"instructions must not be empty"}}`)
		})
		request := Request{Questions: map[string]Question{"q": Noul{}}}
		// when
		result, err := o.ask(t.Context(), request)
		// then
		assert.Nil(t, result)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "422")
	})

	t.Run("returns the api error reported in a successful response", func(t *testing.T) {
		// given
		o := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(t, w, `{"error":{"message":"model is overloaded"}}`)
		})
		request := Request{Questions: map[string]Question{"q": Noul{Instructions: "Is it?"}}}
		// when
		result, err := o.ask(t.Context(), request)
		// then
		assert.Nil(t, result)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "model is overloaded")
	})

	t.Run("returns ErrMissingAnswer when a question goes unanswered", func(t *testing.T) {
		// given
		o := newTestProvider(t, func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(t, w, `{"model":"typesafe/jev-1.13","answers":{},"usage":{}}`)
		})
		request := Request{Questions: map[string]Question{"is_urgent": Noul{Instructions: "Urgent?"}}}
		// when
		result, err := o.ask(t.Context(), request)
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrMissingAnswer)
	})
}

func TestOpenRouterCurrentModel(t *testing.T) {
	t.Run("returns the configured model", func(t *testing.T) {
		// given
		o, err := newOpenRouter(Config{APIKey: "sk-test", Model: "typesafe/jev-1.13"})
		require.NoError(t, err)
		// when
		result := o.currentModel()
		// then
		assert.Equal(t, "typesafe/jev-1.13", result)
	})
}

func newTestProvider(t testing.TB, handler http.HandlerFunc) *openrouter {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	o, err := newOpenRouter(Config{APIKey: "sk-test", Model: "typesafe/jev-1.13", BaseURL: server.URL})
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

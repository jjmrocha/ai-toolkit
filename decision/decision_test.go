package decision

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeProvider struct {
	askFunc          func(context.Context, Request) (*Response, error)
	currentModelFunc func() string
}

func (f fakeProvider) ask(ctx context.Context, req Request) (*Response, error) {
	return f.askFunc(ctx, req)
}

func (f fakeProvider) currentModel() string {
	return f.currentModelFunc()
}

func TestNew(t *testing.T) {
	errorCases := []struct {
		name        string
		cfg         Config
		expectedErr error
	}{
		{
			name:        "missing provider",
			cfg:         Config{Model: "typesafe/jev-1.13"},
			expectedErr: ErrMissingProvider,
		},
		{
			name:        "missing model",
			cfg:         Config{Provider: ProviderOpenRouter},
			expectedErr: ErrMissingModel,
		},
		{
			name:        "unsupported provider",
			cfg:         Config{Provider: "bogus", Model: "typesafe/jev-1.13"},
			expectedErr: ErrUnsupportedProvider,
		},
		{
			name:        "provider construction error propagates",
			cfg:         Config{Provider: ProviderOpenRouter, Model: "typesafe/jev-1.13"},
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

	t.Run("valid openrouter config returns a configured Decision", func(t *testing.T) {
		// given
		cfg := Config{Provider: ProviderOpenRouter, Model: "typesafe/jev-1.13", APIKey: "sk-test"}
		// when
		result, err := New(cfg)
		// then
		require.NoError(t, err)
		assert.Equal(t, "typesafe/jev-1.13", result.CurrentModel())
	})
}

func TestDecisionAsk(t *testing.T) {
	t.Run("delegates to the provider and returns its result", func(t *testing.T) {
		// given
		expected := &Response{Model: "typesafe/jev-1.13-20260917"}
		var gotRequest Request
		d := &Decision{provider: fakeProvider{
			askFunc: func(_ context.Context, req Request) (*Response, error) {
				gotRequest = req
				return expected, nil
			},
		}}
		request := Request{
			State:     "Payouts have been failing for 3 days",
			Questions: map[string]Question{"is_urgent": Noul{Instructions: "Does this convey urgency?"}},
		}
		// when
		result, err := d.Ask(t.Context(), request)
		// then
		require.NoError(t, err)
		assert.Same(t, expected, result)
		assert.Equal(t, request, gotRequest)
	})

	t.Run("times the provider call", func(t *testing.T) {
		// given
		d := &Decision{provider: fakeProvider{
			askFunc: func(context.Context, Request) (*Response, error) {
				time.Sleep(time.Millisecond)
				return &Response{Stats: Stats{InputTokens: 296}}, nil
			},
		}}
		request := Request{Questions: map[string]Question{"q": Noul{Instructions: "Is it?"}}}
		// when
		result, err := d.Ask(t.Context(), request)
		// then
		require.NoError(t, err)
		assert.Equal(t, 296, result.Stats.InputTokens)
		assert.GreaterOrEqual(t, result.Stats.Duration, time.Millisecond)
	})

	t.Run("rejects a request without questions", func(t *testing.T) {
		// given
		d := &Decision{provider: fakeProvider{
			askFunc: func(context.Context, Request) (*Response, error) {
				return nil, errors.New("provider must not be called")
			},
		}}
		// when
		result, err := d.Ask(t.Context(), Request{State: "anything"})
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, ErrNoQuestions)
	})

	t.Run("propagates the provider error", func(t *testing.T) {
		// given
		expectedErr := errors.New("boom")
		d := &Decision{provider: fakeProvider{
			askFunc: func(context.Context, Request) (*Response, error) {
				return nil, expectedErr
			},
		}}
		request := Request{Questions: map[string]Question{"q": Noul{Instructions: "Is it?"}}}
		// when
		result, err := d.Ask(t.Context(), request)
		// then
		assert.Nil(t, result)
		assert.ErrorIs(t, err, expectedErr)
	})
}

func TestDecisionCurrentModel(t *testing.T) {
	t.Run("delegates to the provider", func(t *testing.T) {
		// given
		d := &Decision{provider: fakeProvider{
			currentModelFunc: func() string { return "typesafe/jev-1.13" },
		}}
		// when
		result := d.CurrentModel()
		// then
		assert.Equal(t, "typesafe/jev-1.13", result)
	})
}

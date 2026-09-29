package classify

import (
	"context"
	"fmt"
	"time"
)

// Classifier is a client for one classification model on one provider. Create
// one with [New]. It is safe for concurrent use.
type Classifier struct {
	provider classifierProvider
}

// New creates a [Classifier] for the provider named in cfg. It returns
// [ErrMissingProvider], [ErrMissingModel] or [ErrMissingAPIKey] when those
// fields are empty, and [ErrUnsupportedProvider] for an unknown provider.
func New(cfg Config) (*Classifier, error) {
	if cfg.Provider == "" {
		return nil, ErrMissingProvider
	}

	if cfg.Model == "" {
		return nil, ErrMissingModel
	}

	var provider classifierProvider

	switch cfg.Provider {
	case ProviderOpenRouter:
		p, err := newOpenRouter(cfg)
		if err != nil {
			return nil, err
		}

		provider = p
	default:
		return nil, ErrUnsupportedProvider
	}

	return &Classifier{provider: provider}, nil
}

// Classify asks req.Questions about req.Input and returns one answer per
// question, keyed by the request's identifiers, with the call's usage in
// [Stats]. It returns [ErrNoQuestions] when there are no questions and
// [ErrMissingAnswer] when the provider leaves one unanswered.
func (c *Classifier) Classify(ctx context.Context, req Request) (*Response, error) {
	if len(req.Questions) == 0 {
		return nil, ErrNoQuestions
	}

	for id, q := range req.Questions {
		if q.Type() != YesNoType {
			continue
		}

		question := questionValue[YesNo](q)
		if (question.True == "") != (question.False == "") {
			return nil, fmt.Errorf("%w: yes/no question %q describes one answer, expected both or neither", ErrInvalidQuestion, id)
		}
	}

	start := time.Now()

	resp, err := c.provider.classify(ctx, req)
	if err != nil {
		return nil, err
	}

	resp.Stats.Duration = time.Since(start)

	return resp, nil
}

// CurrentModel returns the model the client uses.
func (c *Classifier) CurrentModel() string {
	return c.provider.currentModel()
}

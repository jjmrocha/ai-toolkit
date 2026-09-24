package classify

import (
	"context"
	"time"
)

// Classifier is a configured client for a single classification model on a single
// provider. Create one with [New]; it is safe for concurrent use.
type Classifier struct {
	provider classifierProvider
}

// New creates a [Classifier] backed by the provider named in cfg. It returns
// [ErrMissingProvider], [ErrMissingModel] or [ErrMissingAPIKey] when those
// fields are empty, and [ErrUnsupportedProvider] when the provider is not
// recognized.
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

// Classify evaluates req.Input against req.Questions and returns one answer per
// question, keyed by the identifiers the request used, and the usage the call
// cost in [Stats]. It returns [ErrNoQuestions] when the request asks nothing
// and [ErrMissingAnswer] when the provider leaves a question unanswered. The
// context controls cancellation and deadline.
func (c *Classifier) Classify(ctx context.Context, req Request) (*Response, error) {
	if len(req.Questions) == 0 {
		return nil, ErrNoQuestions
	}

	start := time.Now()

	resp, err := c.provider.classify(ctx, req)
	if err != nil {
		return nil, err
	}

	resp.Stats.Duration = time.Since(start)

	return resp, nil
}

// CurrentModel returns the identifier of the model the client is configured to
// use.
func (c *Classifier) CurrentModel() string {
	return c.provider.currentModel()
}

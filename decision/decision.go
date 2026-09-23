package decision

import (
	"context"
	"time"
)

// Decision is a configured client for a single decision model on a single
// provider. Create one with [New]; it is safe for concurrent use.
type Decision struct {
	provider decisionProvider
}

// New creates a [Decision] backed by the provider named in cfg. It returns
// [ErrMissingProvider], [ErrMissingModel] or [ErrMissingAPIKey] when those
// fields are empty, and [ErrUnsupportedProvider] when the provider is not
// recognized.
func New(cfg Config) (*Decision, error) {
	if cfg.Provider == "" {
		return nil, ErrMissingProvider
	}

	if cfg.Model == "" {
		return nil, ErrMissingModel
	}

	var provider decisionProvider

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

	return &Decision{provider: provider}, nil
}

// Ask evaluates req.State against req.Questions and returns one answer per
// question, keyed by the identifiers the request used, and the usage the call
// cost in [Stats]. It returns [ErrNoQuestions] when the request asks nothing
// and [ErrMissingAnswer] when the provider leaves a question unanswered. The
// context controls cancellation and deadline.
func (d *Decision) Ask(ctx context.Context, req Request) (*Response, error) {
	if len(req.Questions) == 0 {
		return nil, ErrNoQuestions
	}

	start := time.Now()

	resp, err := d.provider.ask(ctx, req)
	if err != nil {
		return nil, err
	}

	resp.Stats.Duration = time.Since(start)

	return resp, nil
}

// CurrentModel returns the identifier of the model the client is configured to
// use.
func (d *Decision) CurrentModel() string {
	return d.provider.currentModel()
}

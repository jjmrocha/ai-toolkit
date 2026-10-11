package decision

import (
	"context"
	"fmt"
	"time"
)

// Decision is a client for one decision model on one provider. Create
// one with [New]. It is safe for concurrent use.
type Decision struct {
	provider decisionProvider
}

// New creates a [Decision] for the provider named in cfg. It returns
// [ErrMissingProvider], [ErrMissingModel] or [ErrMissingAPIKey] when those
// fields are empty, and [ErrUnsupportedProvider] for an unknown provider.
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

// Ask sends req.Questions about req.State to the model and returns one answer
// per question, keyed by the request's identifiers, with the call's usage in
// [Stats]. It returns [ErrNoQuestions] when there are no questions and
// [ErrMissingAnswer] when the provider leaves one unanswered.
func (d *Decision) Ask(ctx context.Context, req Request) (*Response, error) {
	if len(req.Questions) == 0 {
		return nil, ErrNoQuestions
	}

	for id, q := range req.Questions {
		if err := validateQuestion(id, q); err != nil {
			return nil, err
		}
	}

	start := time.Now()

	resp, err := d.provider.ask(ctx, req)
	if err != nil {
		return nil, err
	}

	resp.Stats.Duration = time.Since(start)

	return resp, nil
}

func validateQuestion(id string, q Question) error {
	switch q.Type() {
	case YesNoType:
		question := questionValue[YesNo](q)
		if (question.True == "") != (question.False == "") {
			return fmt.Errorf("%w: yes/no question %q describes one answer, expected both or neither", ErrInvalidQuestion, id)
		}
	case ChoiceType:
		question := questionValue[Choice](q)
		if len(question.Options) == 0 {
			return fmt.Errorf("%w: choice question %q has no options", ErrInvalidQuestion, id)
		}
	case ScoreType:
		question := questionValue[Score](q)
		if len(question.Levels) == 0 {
			return fmt.Errorf("%w: score question %q has no levels", ErrInvalidQuestion, id)
		}
	}

	return nil
}

// CurrentModel returns the model the client uses.
func (d *Decision) CurrentModel() string {
	return d.provider.currentModel()
}

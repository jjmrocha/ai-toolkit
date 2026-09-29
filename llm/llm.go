package llm

import (
	"context"
	"slices"
	"sync"
)

// LLM is a client for one model on one provider. Create one with [New]. It is
// safe for concurrent use.
type LLM struct {
	config   Config
	provider llmProvider
	mu       sync.RWMutex
}

// New creates an [LLM] for the provider named in cfg. It returns
// [ErrMissingProvider] or [ErrMissingModel] when those fields are empty,
// [ErrUnsupportedProvider] for an unknown provider, and [ErrInvalidEffort] for
// an unknown Config.Effort. An empty Config.Effort means [EffortOff].
func New(cfg Config) (*LLM, error) {
	if cfg.Provider == "" {
		return nil, ErrMissingProvider
	}

	if cfg.Model == "" {
		return nil, ErrMissingModel
	}

	if cfg.Effort == "" {
		cfg.Effort = EffortOff
	}

	if !cfg.Effort.valid() {
		return nil, ErrInvalidEffort
	}

	if !slices.Contains(cfg.Models, cfg.Model) {
		cfg.Models = append([]string{cfg.Model}, cfg.Models...)
	}

	var provider llmProvider

	switch cfg.Provider {
	case ProviderOpenRouter:
		p, err := newOpenRouter(cfg)
		if err != nil {
			return nil, err
		}

		provider = p
	case ProviderOllama:
		p, err := newOllama(cfg)
		if err != nil {
			return nil, err
		}

		provider = p
	case ProviderAnthropic:
		p, err := newAnthropic(cfg)
		if err != nil {
			return nil, err
		}

		provider = p
	default:
		return nil, ErrUnsupportedProvider
	}

	return &LLM{
		config:   cfg,
		provider: provider,
	}, nil
}

// Chat sends messages to the model, offering it tools, and returns its reply.
func (l *LLM) Chat(ctx context.Context, messages []Message, tools []Tool) (*AssistantMessage, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.provider.chat(ctx, messages, tools)
}

// ModelInfo returns the model's provider, display name and context window. It
// returns [ErrModelNotFound] when the provider does not offer the model and
// [ErrMissingContextLength] when the provider reports no context size.
func (l *LLM) ModelInfo(ctx context.Context) (*ModelInfo, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	info, err := l.provider.modelInfo(ctx)
	if err != nil {
		return nil, err
	}

	info.Provider = l.config.Provider
	return info, nil
}

// CurrentModel returns the model the client uses now.
func (l *LLM) CurrentModel() string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.provider.currentModel()
}

// AvailableModels returns the models the client can switch between:
// Config.Models plus Config.Model, which is always included.
func (l *LLM) AvailableModels() []string {
	return l.config.Models
}

// ChangeModel switches the client to model, which must be in
// [LLM.AvailableModels]. It returns [ErrMissingModel] when model is empty and
// [ErrModelNotFound] when it is not available.
func (l *LLM) ChangeModel(model string) error {
	if model == "" {
		return ErrMissingModel
	}

	if !slices.Contains(l.config.Models, model) {
		return ErrModelNotFound
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	return l.provider.changeModel(model)
}

// Effort returns the reasoning effort the client applies to requests.
func (l *LLM) Effort() Effort {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.provider.effort()
}

// ChangeEffort sets the reasoning effort for later requests. An empty e means
// [EffortOff], as in [New]. It returns [ErrInvalidEffort], changing nothing,
// when e is not a known [Effort].
func (l *LLM) ChangeEffort(e Effort) error {
	if e == "" {
		e = EffortOff
	}

	if !e.valid() {
		return ErrInvalidEffort
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.provider.changeEffort(e)

	return nil
}

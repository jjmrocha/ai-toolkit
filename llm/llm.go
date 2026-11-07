package llm

import (
	"context"
	"slices"
	"sync"
)

// LLM is a client for one model on one provider. Create one with [New]. It is
// safe for concurrent use.
type LLM struct {
	config    Config
	provider  llmProvider
	mu        sync.RWMutex
	modelInfo *ModelInfo
}

// New creates an [LLM] for the provider named in cfg. It returns
// [ErrMissingProvider] or [ErrMissingModel] when those fields are empty,
// [ErrUnsupportedProvider] for an unknown provider, and [ErrInvalidEffort] for
// an unknown Config.Effort. An empty Config.Effort means [EffortOff]. New asks
// the provider for the model's info, using ctx, and returns the provider's
// error when that fails, such as [ErrModelNotFound] or
// [ErrMissingContextLength].
func New(ctx context.Context, cfg Config) (*LLM, error) {
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

	cfg.Models = slices.Clone(cfg.Models)
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

	llm := &LLM{
		config:   cfg,
		provider: provider,
	}

	info, err := llm.provider.modelInfo(ctx)
	if err != nil {
		return nil, err
	}

	llm.modelInfo = info

	return llm, nil
}

// Chat sends messages to the model, offering it tools, and returns its reply.
func (l *LLM) Chat(ctx context.Context, messages []Message, tools []Tool) (*AssistantMessage, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.provider.chat(ctx, messages, tools)
}

// ModelInfo returns the provider, id and context window of the model the
// client uses. [New] and [LLM.ChangeModel] fetch it, so ModelInfo sends no
// request. Each call returns a new copy.
func (l *LLM) ModelInfo() *ModelInfo {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return &ModelInfo{
		Provider:    l.modelInfo.Provider,
		Name:        l.modelInfo.Name,
		ContextSize: l.modelInfo.ContextSize,
	}
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
	return slices.Clone(l.config.Models)
}

// ChangeModel switches the client to model, which must be in
// [LLM.AvailableModels]. It returns [ErrMissingModel] when model is empty and
// [ErrModelNotFound] when it is not available. It asks the provider for the new
// model's info, using ctx, and returns it. When that fails, it returns the
// provider's error and keeps the current model. It waits for requests in
// flight to end, and the client's other methods wait with it.
func (l *LLM) ChangeModel(ctx context.Context, model string) (*ModelInfo, error) {
	if model == "" {
		return nil, ErrMissingModel
	}

	if !slices.Contains(l.config.Models, model) {
		return nil, ErrModelNotFound
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	info, err := l.provider.changeModel(ctx, model)
	if err != nil {
		return nil, err
	}

	l.modelInfo = info

	return &ModelInfo{
		Provider:    info.Provider,
		Name:        info.Name,
		ContextSize: info.ContextSize,
	}, nil
}

// Effort returns the reasoning effort the client applies to requests.
func (l *LLM) Effort() Effort {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.provider.effort()
}

// ChangeEffort sets the reasoning effort for later requests. An empty e means
// [EffortOff], as in [New]. It returns [ErrInvalidEffort], changing nothing,
// when e is not a known [Effort]. It waits for requests in flight to end, and
// the client's other methods wait with it.
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

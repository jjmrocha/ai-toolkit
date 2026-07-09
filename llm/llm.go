// Package llm provides a provider-agnostic client for chat-based large language
// models. Construct an [LLM] with [New], then call [LLM.Chat] to exchange
// messages and [LLM.ModelInfo] to query model metadata. OpenRouter, Ollama, and
// Anthropic are the supported providers.
package llm

import (
	"context"
	"slices"
	"sync"
)

// LLM is a configured client for a single model on a single provider. Create
// one with [New]; it is safe for concurrent use.
type LLM struct {
	config   Config
	provider llmProvider
	mu       sync.RWMutex
}

// New creates an [LLM] backed by the provider named in cfg. It returns
// [ErrMissingProvider] or [ErrMissingModel] when those fields are empty,
// [ErrUnsupportedProvider] when the provider is not recognized, and
// [ErrInvalidEffort] when Config.Effort is not a recognized [Effort]. An empty
// Config.Effort defaults to [EffortOff].
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

// Chat sends the conversation in messages to the configured model, optionally
// offering the given tools, and returns the assistant's reply. The context
// controls cancellation and deadline.
func (l *LLM) Chat(ctx context.Context, messages []Message, tools []Tool) (*AssistantMessage, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.provider.chat(ctx, messages, tools)
}

// ModelInfo reports metadata about the configured model: its provider, its
// human-readable name, and its context-window size. It returns [ErrModelNotFound]
// when the provider does not offer the model and [ErrMissingContextLength] when
// the provider reports no context size for it. The context controls
// cancellation and deadline.
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

// CurrentModel returns the identifier of the model the client is currently
// configured to use.
func (l *LLM) CurrentModel() string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.provider.currentModel()
}

// AvailableModels returns the models the client can switch between: the
// identifiers from Config.Models plus the active Config.Model, which [New]
// includes even when Config.Models is empty. The active model is always present.
func (l *LLM) AvailableModels() []string {
	return l.config.Models
}

// ChangeModel switches the client to model, which must be one of
// [LLM.AvailableModels] (the active model always qualifies). It returns
// [ErrMissingModel] when model is empty and [ErrModelNotFound] otherwise.
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

// Effort reports the reasoning effort the client currently applies to requests.
func (l *LLM) Effort() Effort {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.provider.effort()
}

// ChangeEffort sets the reasoning effort applied to subsequent requests.
func (l *LLM) ChangeEffort(e Effort) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.provider.changeEffort(e)
}

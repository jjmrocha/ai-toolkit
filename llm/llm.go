// Package llm provides a provider-agnostic client for chat-based large language
// models. Construct an [LLM] with [New], then call [LLM.Chat] to exchange
// messages and [LLM.ModelInfo] to query model metadata. OpenRouter and Ollama
// are the supported providers.
package llm

import "context"

// LLM is a configured client for a single model on a single provider. Create
// one with [New]; it is safe for concurrent use.
type LLM struct {
	cfg      Config
	provider llmProvider
}

// New creates an [LLM] backed by the provider named in cfg. It returns
// [ErrMissingProvider] or [ErrMissingModel] when those fields are empty, and
// [ErrUnsupportedProvider] when the provider is not recognized.
func New(cfg Config) (*LLM, error) {
	if cfg.Provider == "" {
		return nil, ErrMissingProvider
	}

	if cfg.Model == "" {
		return nil, ErrMissingModel
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
	default:
		return nil, ErrUnsupportedProvider
	}

	return &LLM{
		cfg:      cfg,
		provider: provider,
	}, nil
}

// Chat sends the conversation in messages to the configured model, optionally
// offering the given tools, and returns the assistant's reply. The context
// controls cancellation and deadline.
func (l *LLM) Chat(ctx context.Context, messages []Message, tools []Tool) (*AssistantMessage, error) {
	return l.provider.chat(ctx, messages, tools)
}

// ModelInfo reports metadata about the configured model, such as its
// human-readable name and context-window size. It returns [ErrModelNotFound]
// when the provider does not offer the model and [ErrMissingContextLength] when
// the provider reports no context size for it. The context controls
// cancellation and deadline.
func (l *LLM) ModelInfo(ctx context.Context) (*ModelInfo, error) {
	return l.provider.modelInfo(ctx)
}

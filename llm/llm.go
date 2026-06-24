package llm

import "context"

type LLM struct {
	cfg      Config
	provider llmProvider
}

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
	default:
		return nil, ErrUnsupportedProvider
	}

	return &LLM{
		cfg:      cfg,
		provider: provider,
	}, nil
}

func (l *LLM) Chat(ctx context.Context, messages []Message, tools []Tool) (*AssistantMessage, error) {
	return l.provider.chat(ctx, messages, tools)
}

func (l *LLM) ModelInfo(ctx context.Context) (ModelInfo, error) {
	return l.provider.modelInfo(ctx)
}

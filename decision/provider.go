package decision

import "context"

// Provider identifies a decision backend.
type Provider string

// ProviderOpenRouter selects the OpenRouter backend (https://openrouter.ai).
const ProviderOpenRouter Provider = "openrouter"

type decisionProvider interface {
	ask(context.Context, Request) (*Response, error)
	currentModel() string
}

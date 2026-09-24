package classify

import "context"

// Provider identifies a supported classification backend.
type Provider string

// ProviderOpenRouter selects the OpenRouter backend (https://openrouter.ai),
// which serves TypeSafe's Jev models.
const ProviderOpenRouter Provider = "openrouter"

type classifierProvider interface {
	classify(context.Context, Request) (*Response, error)
	currentModel() string
}

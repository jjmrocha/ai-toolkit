package decision

import "context"

// Provider identifies a supported decision backend.
type Provider string

// ProviderOpenRouter selects the OpenRouter backend (https://openrouter.ai),
// which serves TypeSafe's Jev models.
const ProviderOpenRouter Provider = "openrouter"

type decisionProvider interface {
	ask(context.Context, Request) (*Response, error)
	currentModel() string
}

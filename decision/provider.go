package decision

import "context"

// Provider identifies a decision backend.
type Provider string

// ProviderOpenRouter selects the OpenRouter backend (https://openrouter.ai).
const ProviderOpenRouter Provider = "openrouter"

// ProviderOllama selects a local Ollama server (https://ollama.com). It needs
// no API key: Config.APIKey is ignored.
const ProviderOllama Provider = "ollama"

type decisionProvider interface {
	ask(context.Context, Request) (*Response, error)
	currentModel() string
}

package llm

import "context"

// Provider identifies a supported LLM backend.
type Provider string

const (
	// ProviderOpenRouter selects the OpenRouter backend (https://openrouter.ai).
	ProviderOpenRouter Provider = "openrouter"
	// ProviderOllama selects an Ollama server, local or remote (https://ollama.com).
	ProviderOllama Provider = "ollama"
	// ProviderAnthropic selects the Anthropic backend (https://www.anthropic.com).
	ProviderAnthropic Provider = "anthropic"
)

type llmProvider interface {
	chat(context.Context, []Message, []Tool) (*AssistantMessage, error)
	modelInfo(context.Context) (*ModelInfo, error)
	changeModel(model string) error
	currentModel() string
	effort() Effort
	changeEffort(e Effort)
}

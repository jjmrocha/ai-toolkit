package llm

// Provider identifies a supported LLM backend.
type Provider string

const (
	// ProviderOpenRouter selects the OpenRouter backend (https://openrouter.ai).
	ProviderOpenRouter Provider = "openrouter"
	// ProviderOllama selects a local or remote Ollama backend (https://ollama.com).
	ProviderOllama Provider = "ollama"
)

// Config configures an [LLM]. Provider, APIKey, and Model are required; BaseURL
// defaults to the provider's standard endpoint when empty.
type Config struct {
	// Provider selects the LLM backend. Required.
	Provider Provider
	// BaseURL overrides the provider's default API endpoint. Optional.
	BaseURL string
	// APIKey authenticates requests to the provider. Required.
	APIKey string `json:"-"`
	// Model is the provider-specific model identifier, e.g. "openai/gpt-4o". Required.
	Model string
}

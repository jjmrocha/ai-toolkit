package llm

// Provider identifies a supported LLM backend.
type Provider string

const (
	// ProviderOpenRouter selects the OpenRouter backend (https://openrouter.ai).
	ProviderOpenRouter Provider = "openrouter"
	// ProviderOllama selects a local or remote Ollama backend (https://ollama.com).
	ProviderOllama Provider = "ollama"
	// ProviderAnthropic selects the Anthropic backend (https://www.anthropic.com).
	ProviderAnthropic Provider = "anthropic"
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
	// Model name selects the LLM model to use. Required.
	Model string
	// Models lists the available LLM models for the provider. Optional.
	Models []string
	// MaxTokens caps the tokens generated per response. When zero, OpenRouter
	// and Ollama omit the cap and let the model decide, while Anthropic (which
	// requires the field) applies its own default.
	MaxTokens int
}

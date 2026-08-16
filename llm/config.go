package llm

// Config configures an [LLM]. Provider and Model are always required; APIKey is
// required for OpenRouter and Anthropic but not for Ollama. BaseURL defaults to
// the provider's standard endpoint when empty.
type Config struct {
	// Provider selects the LLM backend. Required.
	Provider Provider
	// BaseURL overrides the provider's default API endpoint. Optional.
	BaseURL string
	// APIKey authenticates requests to the provider. Required for OpenRouter and
	// Anthropic; unused by Ollama.
	APIKey string `json:"-"`
	// Model name selects the LLM model to use. Required.
	Model string
	// Models lists the available LLM models for the provider. Optional.
	Models []string
	// MaxTokens caps the tokens generated per response. When zero, OpenRouter
	// and Ollama omit the cap and let the model decide, while Anthropic (which
	// requires the field) applies its own default.
	MaxTokens int
	// Effort controls how much reasoning the model does before answering.
	// Optional; when empty it defaults to [EffortOff].
	Effort Effort
}

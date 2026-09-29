package llm

// Config configures an [LLM]. Provider and Model are required. APIKey is
// required for OpenRouter and Anthropic, not for Ollama.
type Config struct {
	// Provider selects the LLM backend. Required.
	Provider Provider
	// BaseURL overrides the provider's default endpoint. Optional.
	BaseURL string
	// APIKey authenticates requests to the provider. Required for OpenRouter and
	// Anthropic; unused by Ollama.
	APIKey string `json:"-"`
	// Model is the model to use. Required.
	Model string
	// Models lists other models [LLM.ChangeModel] may switch to. Optional.
	Models []string
	// MaxTokens caps the tokens generated per response. When zero, OpenRouter and
	// Ollama send no cap, and Anthropic, which requires one, uses its own default.
	MaxTokens int
	// Effort sets how much the model reasons before answering. Empty means
	// [EffortOff].
	Effort Effort
}

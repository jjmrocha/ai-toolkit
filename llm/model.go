package llm

// ModelInfo describes a model offered by a provider.
type ModelInfo struct {
	// Provider is the model's provider.
	Provider Provider
	// Name is the model's id, as in Config.Model.
	Name string
	// ContextSize is the model's context window, in tokens.
	ContextSize int
}

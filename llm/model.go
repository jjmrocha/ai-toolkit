package llm

// ModelInfo describes a model offered by a provider.
type ModelInfo struct {
	// Name is the model's human-readable name.
	Name string
	// ContextSize is the model's maximum context window, in tokens.
	ContextSize int
}

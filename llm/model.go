package llm

// ModelInfo describes a model offered by a provider.
type ModelInfo struct {
	// Provider is the model's provider.
	Provider Provider
	// Name is the model's id, as in Config.Model.
	Name string
	// ContextSize is the model's context window, in tokens.
	ContextSize int
	// Capabilities lists the features the model supports.
	Capabilities Capabilities
}

// Capabilities tells which features a model supports. A false field means the
// provider did not report the feature.
type Capabilities struct {
	// Tools is true when the model can call the tools passed to [LLM.Chat].
	Tools bool
	// Reasoning is true when the model can think before it answers.
	Reasoning bool
	// Images is true when the model accepts images as input.
	Images bool
}

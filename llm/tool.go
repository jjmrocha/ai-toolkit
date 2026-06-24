package llm

// Tool describes a function the model may call. It is sent to the provider as a
// tool definition and does not execute anything itself.
type Tool struct {
	// Name is the function name the model uses to invoke the tool.
	Name string
	// Description tells the model what the tool does and when to use it.
	Description string
	// Schema is the JSON Schema describing the tool's parameters.
	Schema map[string]any
}

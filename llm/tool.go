package llm

const typeFunction = "function"

// Tool describes a function the model may call. It is sent to the provider as a
// definition and runs nothing itself.
type Tool struct {
	// Name is the name the model calls the tool by.
	Name string
	// Description tells the model what the tool does and when to use it.
	Description string
	// Schema is the JSON Schema of the tool's parameters.
	Schema map[string]any
}

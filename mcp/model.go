package mcp

// Status is one registered MCP and whether it is running, as returned by
// [Manager.Status].
type Status struct {
	// Name is the MCP's registered name.
	Name string
	// Active is true while the MCP's process is running.
	Active bool
}

// Instruction is usage guidance for a model, labeled with the name of the MCP
// or pack it came from.
type Instruction struct {
	// Name is the MCP's registered name.
	Name string
	// Text is the guidance, empty when there is none.
	Text string
}

type toolSpec struct {
	name        string
	description string
	schema      map[string]any
}

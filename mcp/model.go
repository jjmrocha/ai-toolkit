package mcp

// Status reports whether a registered MCP is currently running, as returned by
// [Manager.Status].
type Status struct {
	// Name is the MCP's registered name.
	Name string
	// Active is true while the MCP's server process is running.
	Active bool
}

// Instruction pairs an MCP's registered name with the usage instructions it
// sent as part of its initialize handshake, as returned by
// [Client.Instructions] and [Manager.Instructions].
type Instruction struct {
	// Name is the MCP's registered name.
	Name string
	// Text is the instructions the server sent at handshake, empty when it
	// sent none.
	Text string
}

type toolSpec struct {
	name        string
	description string
	schema      map[string]any
}

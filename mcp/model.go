package mcp

// Status reports whether a registered MCP is currently running.
type Status struct {
	// Name is the MCP's registered name.
	Name string
	// Active is true while the MCP's server process is running.
	Active bool
}

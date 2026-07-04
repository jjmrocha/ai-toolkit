package mcp

// ClientConfig describes the MCP server a Client launches. Command and Args are
// run with os/exec without a shell, so they are trusted input: supply them from
// operator configuration, never from an untrusted source.
type ClientConfig struct {
	// Name namespaces this server's tools in the ToolBox as "<Name>__<tool>".
	Name string
	// Command is the server executable to launch.
	Command string
	// Args are the arguments passed to Command.
	Args []string
}

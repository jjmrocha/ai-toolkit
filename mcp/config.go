package mcp

import "time"

// ClientConfig describes the MCP server a [Client] launches. Command and Args
// are run with os/exec without a shell, so they are trusted input: supply them
// from operator configuration, never from an untrusted source.
type ClientConfig struct {
	// Name namespaces this server's tools in the ToolBox as "<Name>__<tool>".
	Name string
	// Command is the server executable to launch.
	Command string
	// Args are the arguments passed to Command.
	Args []string
	// ToolCallTimeout bounds a single call to one of this server's tools,
	// measured from the moment the call is sent. It is a ceiling within the
	// caller's own context, so a server that goes quiet fails that one call and
	// leaves the caller's deadline intact. Zero or less selects a default of two
	// minutes.
	ToolCallTimeout time.Duration
}

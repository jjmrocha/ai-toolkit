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
	// InheritEnv names environment variables copied from the calling process
	// into the server's environment, on top of the ones every server receives:
	// HOME, LOGNAME, PATH, SHELL, TERM, USER, TMPDIR, LANG, TZ, SSL_CERT_DIR,
	// SSL_CERT_FILE, the proxy variables in both cases, and every LC_ variable.
	// Nothing else is passed on, so a server does not receive the credentials
	// the calling process holds in its environment; name here the ones a
	// particular server needs. A name the calling process does not set is
	// skipped rather than passed on empty.
	InheritEnv []string
	// ToolCallTimeout bounds a single call to one of this server's tools,
	// measured from the moment the call is sent. It is a ceiling within the
	// caller's own context, so a server that goes quiet fails that one call and
	// leaves the caller's deadline intact. Zero or less selects a default of two
	// minutes.
	ToolCallTimeout time.Duration
}

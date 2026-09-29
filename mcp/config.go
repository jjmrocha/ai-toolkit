package mcp

import "time"

// ClientConfig describes the MCP server a [Client] launches. Command and Args
// run through os/exec with no shell, so they are trusted input: take them from
// operator configuration, never from an untrusted source.
type ClientConfig struct {
	// Name namespaces this server's tools in the ToolBox as "<Name>__<tool>".
	Name string
	// Command is the server executable to launch.
	Command string
	// Args are the arguments passed to Command.
	Args []string
	// ExcludedTools names server tools to leave unregistered, by the name the
	// server publishes, not the namespaced one. The filter applies every time the
	// tool list is read, so an excluded tool stays out when the server changes its
	// list. Names the server never publishes are ignored.
	ExcludedTools []string
	// InheritEnv names environment variables to copy from the calling process to
	// the server. Every server already gets HOME, LOGNAME, PATH, SHELL, TERM, USER,
	// TMPDIR, LANG, TZ, SSL_CERT_DIR, SSL_CERT_FILE, the proxy variables in either
	// case, and every LC_ variable. Nothing else is passed, so the server does not
	// see credentials in the caller's environment unless they are named here. A
	// name the caller does not set is skipped, not passed empty.
	InheritEnv []string
	// ToolCallTimeout limits one call to one of the server's tools. It is an idle
	// timeout: each progress notification from the server restarts it, so a tool
	// that keeps reporting progress keeps running, and one that goes quiet fails
	// that call. It runs inside the caller's context and leaves the caller's
	// deadline alone. Zero or less means sixty seconds.
	ToolCallTimeout time.Duration
}

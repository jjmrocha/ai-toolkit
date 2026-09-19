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
	// ExcludedTools names tools this server publishes that are not to be
	// registered, by the name the server publishes them under rather than the
	// namespaced one. The filter applies every time the tool list is read, so a
	// tool named here stays unregistered when the server announces a change to
	// its list. A name the server never publishes is ignored.
	ExcludedTools []string
	// InheritEnv names environment variables copied from the calling process
	// into the server's environment, on top of the ones every server receives:
	// HOME, LOGNAME, PATH, SHELL, TERM, USER, TMPDIR, LANG, TZ, SSL_CERT_DIR,
	// SSL_CERT_FILE, the proxy variables in both cases, and every LC_ variable.
	// Nothing else is passed on, so a server does not receive the credentials
	// the calling process holds in its environment; name here the ones a
	// particular server needs. A name the calling process does not set is
	// skipped rather than passed on empty.
	InheritEnv []string
	// ToolCallTimeout bounds a single call to one of this server's tools. It is
	// an idle timeout rather than a total budget: every progress notification the
	// server sends restarts the clock, so a tool that reports progress runs as
	// long as it keeps reporting, while one that goes quiet fails that one call.
	// It applies within the caller's own context, so a failed call leaves the
	// caller's deadline intact. Zero or less selects a default of sixty seconds.
	ToolCallTimeout time.Duration
}

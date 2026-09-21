package mcp

import (
	"errors"
)

// Errors reported by the mcp package.
var (
	// ErrNameRequired is returned by [NewClient] when ClientConfig.Name is empty.
	ErrNameRequired = errors.New("MCP name is required")
	// ErrCommandRequired is returned by [NewClient] when ClientConfig.Command is
	// empty.
	ErrCommandRequired = errors.New("MCP command is required")
	// ErrMCPNotRegistered is returned by [Manager.Start] and [Manager.Stop] when
	// no MCP has been registered under the given name.
	ErrMCPNotRegistered = errors.New("MCP not registered")
	// ErrRequestTimeout is the cancellation cause recorded against a tool call
	// that sends no progress for longer than [ClientConfig.ToolCallTimeout],
	// which aborts that one call and leaves the caller's own context untouched.
	// No function returns it: the SDK reports an aborted call as
	// context.Canceled, so that is what the tool handler returns, and
	// context.Cause recovers this error only from the context the handler used
	// internally. A caller therefore cannot tell a quiet server from a
	// cancellation.
	ErrRequestTimeout = errors.New("request timeout")
)

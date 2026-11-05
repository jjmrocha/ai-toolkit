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
	// ErrMCPNotRegistered is returned by [Manager.Start] and [Manager.Stop] when no
	// MCP is registered under the name.
	ErrMCPNotRegistered = errors.New("MCP not registered")
	// ErrToolNameTaken is returned by [NewClient] when
	// [ClientConfig.NoToolPrefix] is set and one of the server's tool names is
	// already registered in the ToolBox.
	ErrToolNameTaken = errors.New("tool name already taken")
	// ErrRequestTimeout is the cancellation cause set on a tool call that sends no
	// progress for longer than [ClientConfig.ToolCallTimeout]. No function returns
	// it: the SDK reports the aborted call as context.Canceled, which is what the
	// tool handler returns, and only the handler's internal context carries this
	// cause. A caller cannot tell a quiet server from a cancellation.
	ErrRequestTimeout = errors.New("request timeout")
)

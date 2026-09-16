package mcp

import (
	"errors"
)

// Errors returned by the mcp package.
var (
	// ErrNameRequired is returned by [NewClient] when ClientConfig.Name is empty.
	ErrNameRequired = errors.New("MCP name is required")
	// ErrCommandRequired is returned by [NewClient] when ClientConfig.Command is
	// empty.
	ErrCommandRequired = errors.New("MCP command is required")
	// ErrMCPNotRegistered is returned by [Manager.Start] and [Manager.Stop] when
	// no MCP has been registered under the given name.
	ErrMCPNotRegistered = errors.New("MCP not registered")
)

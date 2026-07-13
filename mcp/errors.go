package mcp

import "errors"

var (
	// ErrNameRequired is returned by NewClient when ClientConfig.Name is empty.
	ErrNameRequired = errors.New("MCP name is required")
	// ErrCommandRequired is returned by NewClient when ClientConfig.Command is empty.
	ErrCommandRequired = errors.New("MCP command is required")
	// ErrAlreadyRegistered is returned by RegisterTools when the client has
	// already registered its tools in a ToolBox.
	ErrAlreadyRegistered = errors.New("MCP tools already registered")
	// ErrMCPConnectionClosed is returned when the server closes its stdout
	// before answering a request.
	ErrMCPConnectionClosed = errors.New("MCP connection closed")
	// ErrUnsupportedProtocolVersion is returned during the handshake when the
	// server omits its protocol version or offers one the client does not support.
	ErrUnsupportedProtocolVersion = errors.New("unsupported MCP protocol version")
	// ErrMCPNotRegistered is returned by Manager.Start and Manager.Stop when no
	// MCP has been registered under the given name.
	ErrMCPNotRegistered = errors.New("MCP not registered")
)

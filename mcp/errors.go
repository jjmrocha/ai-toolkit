package mcp

import "errors"

var (
	// ErrNameRequired is returned by NewClient when ClientConfig.Name is empty.
	ErrNameRequired = errors.New("MCP name is required")
	// ErrCommandRequired is returned by NewClient when ClientConfig.Command is empty.
	ErrCommandRequired = errors.New("MCP command is required")
	// ErrMCPConnectionClosed is returned when the server closes its stdout
	// before answering a request.
	ErrMCPConnectionClosed = errors.New("MCP connection closed")
	// ErrUnsupportedProtocolVersion is returned during the handshake when the
	// server omits its protocol version or offers one the client does not support.
	ErrUnsupportedProtocolVersion = errors.New("unsupported MCP protocol version")
)

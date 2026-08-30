package mcp

import (
	"errors"

	"github.com/jjmrocha/ai-toolkit/internal/helper"
)

// Errors returned by the mcp package.
var (
	// ErrNameRequired is returned by [NewClient] when ClientConfig.Name is empty.
	ErrNameRequired = errors.New("MCP name is required")
	// ErrCommandRequired is returned by [NewClient] when ClientConfig.Command is
	// empty.
	ErrCommandRequired = errors.New("MCP command is required")
	// ErrAlreadyRegistered is returned by [Client.RegisterTools] when the client
	// has already registered its tools in a ToolBox.
	ErrAlreadyRegistered = errors.New("MCP tools already registered")
	// ErrMCPConnectionClosed is returned by any request to the server — the
	// [NewClient] handshake, [Client.RegisterTools], or a registered tool's
	// handler — when the server closes its stdout before answering.
	ErrMCPConnectionClosed = errors.New("MCP connection closed")
	// ErrUnsupportedProtocolVersion is returned by [NewClient] when the server
	// omits its protocol version during the handshake or offers one the client
	// does not support.
	ErrUnsupportedProtocolVersion = errors.New("unsupported MCP protocol version")
	// ErrTooManyToolPages is returned by [Client.RegisterTools] when the server
	// keeps handing out cursors past the page limit instead of ending the tool
	// list.
	ErrTooManyToolPages = errors.New("too many MCP tool pages")
	// ErrMCPNotRegistered is returned by [Manager.Start] and [Manager.Stop] when
	// no MCP has been registered under the given name.
	ErrMCPNotRegistered = errors.New("MCP not registered")
	// ErrProcessClosed is returned by any request to the server — the
	// [NewClient] handshake, [Client.RegisterTools], or a registered tool's
	// handler — when the server process has already stopped.
	ErrProcessClosed = helper.ErrProcessClosed
	// ErrInvalidMessage is returned by any request to the server — the
	// [NewClient] handshake, [Client.RegisterTools], or a registered tool's
	// handler — when the message cannot be framed for the wire as a single line.
	ErrInvalidMessage = helper.ErrInvalidMessage
)

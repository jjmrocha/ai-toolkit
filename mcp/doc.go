// Package mcp connects stdio-based MCP (Model Context Protocol) servers to a
// ToolBox from the tools package. A [Client] launches one server as a child
// process, discovers the tools it offers, and registers each one in the ToolBox
// so a model can call them like any other tool. A [Manager] holds the
// configuration of several servers and starts and stops them by name against a
// shared ToolBox.
//
// A Client drives exactly one server over its stdin/stdout, so several calls may
// be in flight at a time. A call that outlives [ClientConfig.ToolCallTimeout]
// without the server reporting progress fails on its own and leaves the caller's
// deadline intact. A server that announces a change to its tool list has those
// tools registered again without the caller asking.
package mcp

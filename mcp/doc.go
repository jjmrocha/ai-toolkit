// Package mcp connects stdio-based MCP (Model Context Protocol) servers to a
// ToolBox from the tools package. A [Client] launches one server as a child
// process, discovers the tools it offers, and registers each one in the ToolBox
// so a model can call them like any other tool. A [Manager] holds the
// configuration of several servers and starts and stops them by name against a
// shared ToolBox.
//
// A Client drives exactly one server over its stdin/stdout. Requests are matched
// to responses by id, so several may be in flight at a time. A request blocked
// on a silent server returns when its context is cancelled or its deadline
// expires; a response arriving after that is discarded.
package mcp

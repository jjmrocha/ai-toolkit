// Package mcp connects stdio MCP (Model Context Protocol) servers to a
// [tools.ToolBox]. A [Client] launches one server as a child process, lists its
// tools, and registers each one in the ToolBox, so a model calls them like any
// other tool. A [Manager] holds the configuration of several servers and starts
// and stops them by name against one ToolBox.
//
// A Client talks to one server over its stdin and stdout, with several calls in
// flight at once. A call that goes quiet for longer than
// [ClientConfig.ToolCallTimeout] fails on its own, leaving the caller's
// deadline alone. When the server changes its tool list, the Client registers
// the tools again without being asked.
package mcp

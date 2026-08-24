// Package tools helps wire model tool calls to Go code. It provides a [ToolBox]
// that pairs each [llm.Tool] definition with the [Handler] that runs it, an
// [ObjectBuilder] for constructing the JSON Schema that describes a tool's
// parameters without hand-writing nested maps, and an [Arguments] wrapper for
// reading a call's decoded arguments back out with typed accessors.
package tools

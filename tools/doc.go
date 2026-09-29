// Package tools connects model tool calls to Go code. A [ToolBox] pairs each
// [llm.Tool] definition with the [Handler] that runs it. An [ObjectBuilder]
// builds the JSON Schema for a tool's parameters, and [Arguments] reads a
// call's decoded arguments with typed accessors.
package tools

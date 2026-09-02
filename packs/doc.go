// Package packs bundles tools that arrive ready to use. One call registers a
// bundle in a ToolBox from the tools package and hands back a [ToolPack], which
// stops whatever serves the bundle and takes its tools out again.
//
// [WebTools] is the web pack: search, page fetching and site crawling.
// [CodingTools] is the coding pack: symbol-aware navigation and editing,
// diagnostics, files and shell. Both are served by an MCP server the pack
// launches and owns.
//
// [ShellTools] runs a command line with /bin/sh, and [FileTools] reads, writes,
// edits, lists and deletes files under one folder it cannot leave. The program
// serves both itself, so their ToolPack has no server to stop and only
// unregisters.
package packs

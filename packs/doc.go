// Package packs bundles tools that arrive ready to use. One call registers a
// bundle in a ToolBox from the tools package and hands back a [ToolPack], which
// stops whatever serves the bundle and takes its tools out again.
//
// [WebTools] is the web pack: search, page fetching and site crawling.
// [CodingTools] is the coding pack: symbol-aware navigation and editing,
// diagnostics, files and read-only queries against other projects. Both are
// served by an MCP server the pack launches and owns.
//
// [ShellTools] runs a command line with /bin/sh, which the coding pack leaves
// out, so an agent that builds or runs what it wrote loads both. [FileTools]
// reads, writes, edits, lists, searches and deletes files under one folder it
// cannot leave, and [DateTools] tells the model the date, the time of day and
// the host's zone. The program serves all three itself, so their ToolPack has
// no server to stop and only unregisters.
//
// [DecisionTools] hands one judgement call at a time to a decision model the
// caller supplies, and returns calibrated probabilities instead of prose. Its
// ToolPack only unregisters too: the client stays the caller's.
package packs

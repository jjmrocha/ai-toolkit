// Package packs registers ready-made tool bundles in a [tools.ToolBox]. Each
// constructor returns a [ToolPack] whose Close removes the tools again and stops
// whatever serves them.
//
// [WebTools] (web search, page fetching, site crawling) and [CodingTools]
// (symbol-aware code navigation and editing) are served by an MCP server the pack
// launches and owns.
//
// [ShellTools], [FileTools] and [DateTools] run inside the program, so their Close
// only unregisters. [ShellTools] is the shell the coding pack leaves out.
// [FileTools] works on files under one folder it cannot leave. [DateTools] gives
// the model the date, the time of day and the host's zone.
//
// [ClassifyTools] sends one judgement call at a time to a classification model
// the caller supplies, and returns calibrated probabilities. Its Close also only
// unregisters; the client stays the caller's.
package packs

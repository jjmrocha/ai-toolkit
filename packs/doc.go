// Package packs registers ready-made tool bundles in a [tools.ToolBox]. Each
// constructor returns a [ToolPack] whose Close removes the tools again and stops
// whatever serves them.
//
// [WebTools] (web search, page fetching, site crawling), [CodingTools]
// (symbol-aware code navigation and editing), [ThinkingTools] (step-by-step
// reasoning the model can revise and branch), [LibraryTools] (current
// documentation for libraries and frameworks) and [FinanceTools] (market data
// from Yahoo Finance) are served by an MCP server the pack launches and owns.
//
// [ShellTools], [FileTools] and [DateTools] run inside the program, so their Close
// only unregisters. [ShellTools] is the shell the coding pack leaves out.
// [FileTools] works on files under one folder it cannot leave. [DateTools] gives
// the model the date, the time of day and the host's zone.
//
// [DecisionTools] sends one judgement call at a time to a decision model
// the caller supplies, and returns probabilities. Its Close also only
// unregisters; the client stays the caller's.
//
// [SubAgentTools] hands a task to one of the agents in a [subagent.Manager] the
// caller supplies, and returns the child's final answer. Its Close only
// unregisters too; the manager stays the caller's.
package packs

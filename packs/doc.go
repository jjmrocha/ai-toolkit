// Package packs bundles tools that arrive ready to use. One call registers a
// bundle in a ToolBox from the tools package and hands back a [ToolPack], which
// stops whatever serves the bundle and takes its tools out again.
//
// [WebTools] is the web pack: search, page fetching and site crawling.
// [CodingTools] is the coding pack: symbol-aware navigation and editing,
// diagnostics, files and shell. Each is served by an MCP server the pack
// launches and owns.
package packs

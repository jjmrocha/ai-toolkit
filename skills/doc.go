// Package skills gives a model instructions it loads only when it needs them. A
// [Collection] holds the skills a session may use, added one folder at a time
// with [Collection.Add]. Each folder has a SKILL.md: frontmatter with the
// skill's name and description, then the instructions.
//
// Only names and descriptions reach the model up front, in the catalog an agent
// appends to its system message (see [Collection.Catalog]). The model gets the
// rest through the three tools [Collection.RegisterTools] adds: "skill_load"
// returns a skill's instructions and the list of files it ships,
// "skill_load_file" returns one of those files, and "skill_execute_file" runs
// one and returns its output.
//
// A script run by "skill_execute_file" is stopped after two minutes, unless the
// caller's context already has a deadline, which is kept.
//
// A skill folder is trusted input, like an MCP server command: what
// "skill_execute_file" runs has the program's authority and inherits its
// environment, credentials included.
//
// Nothing is discovered automatically. A skill reaches a model only because the
// caller added its folder.
package skills

// Package skills gives a model instructions it loads only when it needs them. A
// [Collection] holds the skills a session may use; the caller fills it with
// [Collection.Add], one folder at a time. Each folder holds a SKILL.md whose
// frontmatter carries the skill's name and description and whose body holds the
// instructions.
//
// Only names and descriptions reach the model up front, as the catalog an agent
// appends to its system message (see [Collection.Catalog]). The bodies stay out
// of the conversation until the model asks for one, through the two tools
// [Collection.RegisterTools] adds to a ToolBox: [loadToolName] returns a skill's
// instructions together with the list of files that skill ships, and
// [loadFileToolName] returns one of those files.
//
// Nothing is discovered automatically. A skill reaches a model only because the
// caller added its folder.
package skills

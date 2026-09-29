package skills

import "errors"

// Errors returned by the skills package.
var (
	// ErrSkillFolderNotFound is returned by [Collection.Add] when the path does
	// not exist or is not a directory.
	ErrSkillFolderNotFound = errors.New("skill folder not found")

	// ErrNoSkillFile is returned by [Collection.Add] when the folder holds no
	// SKILL.md.
	ErrNoSkillFile = errors.New("SKILL.md not found")

	// ErrInvalidFrontmatter is returned by [Collection.Add] when SKILL.md does not
	// start with a --- fence, never closes it, or fences something other than a
	// YAML mapping of scalars.
	ErrInvalidFrontmatter = errors.New("invalid skill frontmatter")

	// ErrNameRequired is returned by [Collection.Add] when the frontmatter has no
	// name.
	ErrNameRequired = errors.New("skill name is required")

	// ErrDescriptionRequired is returned by [Collection.Add] when the frontmatter
	// has no description.
	ErrDescriptionRequired = errors.New("skill description is required")

	// ErrDuplicateSkill is returned by [Collection.Add] when a skill with the same
	// name was already added.
	ErrDuplicateSkill = errors.New("skill already added")

	// ErrInvalidSkillName is returned by [Collection.AddClaudeSkill] when the name
	// is not a single folder inside the Claude skills folder.
	ErrInvalidSkillName = errors.New("invalid skill name")

	// ErrSkillNotFound is returned by the "skill_load", "skill_load_file" and
	// "skill_execute_file" tools when no skill has the requested name.
	ErrSkillNotFound = errors.New("skill not found")

	// ErrFileNotFound is returned by the "skill_load_file" and "skill_execute_file"
	// tools when the skill ships no file at the requested path.
	ErrFileNotFound = errors.New("file not found")
)

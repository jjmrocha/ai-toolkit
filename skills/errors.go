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

	// ErrInvalidFrontmatter is returned by [Collection.Add] when SKILL.md does
	// not open with a --- fence, never closes it, or holds a value the parser
	// cannot read, such as a folded or literal block scalar.
	ErrInvalidFrontmatter = errors.New("invalid skill frontmatter")

	// ErrNameRequired is returned by [Collection.Add] when the frontmatter
	// carries no name.
	ErrNameRequired = errors.New("skill name is required")

	// ErrDescriptionRequired is returned by [Collection.Add] when the
	// frontmatter carries no description.
	ErrDescriptionRequired = errors.New("skill description is required")

	// ErrDuplicateSkill is returned by [Collection.Add] when a skill with the
	// same name has already been added.
	ErrDuplicateSkill = errors.New("skill already added")

	// ErrSkillNotFound is returned by the [loadToolName] and [loadFileToolName]
	// tools when no skill in the collection matches the requested name.
	ErrSkillNotFound = errors.New("skill not found")

	// ErrFileNotFound is returned by the [loadFileToolName] tool when the skill
	// ships no file under the requested path.
	ErrFileNotFound = errors.New("file not found")
)

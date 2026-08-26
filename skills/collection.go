package skills

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

const (
	loadToolName = "skill_load"
	skillFile    = "SKILL.md"
)

type skill struct {
	name        string
	description string
	body        string
	skillPath   string
	files       []string
}

// Collection holds the skills a session may load. Fill it with [Collection.Add]
// before handing it to an agent; each skill is a folder containing a SKILL.md
// whose frontmatter carries the skill's name and description.
//
// A Collection is safe for concurrent use.
type Collection struct {
	mu     sync.RWMutex
	skills map[string]skill
}

// NewCollection returns an empty [Collection] ready for [Collection.Add].
func NewCollection() *Collection {
	return &Collection{
		skills: make(map[string]skill),
	}
}

// Add reads path/SKILL.md and registers the skill it describes under the name
// from its frontmatter. The rest of the folder is listed so the model can read
// those files later, and the listing is taken once, here. A relative path is
// resolved against the working directory as it stands now, so a later chdir
// does not move the skill.
//
// It returns [ErrSkillFolderNotFound] when path is not a directory,
// [ErrNoSkillFile] when the folder holds no SKILL.md, [ErrInvalidFrontmatter]
// when that file's frontmatter cannot be read, [ErrNameRequired] or
// [ErrDescriptionRequired] when the frontmatter omits either, and
// [ErrDuplicateSkill] when the name is already taken.
func (c *Collection) Add(path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("%w: %q", ErrSkillFolderNotFound, path)
	}

	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("%w: %q", ErrSkillFolderNotFound, path)
	}

	root, err := os.OpenRoot(path)
	if err != nil {
		return fmt.Errorf("%w: %q", ErrSkillFolderNotFound, path)
	}

	defer func() { _ = root.Close() }()

	content, err := fs.ReadFile(root.FS(), skillFile)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: %q", ErrNoSkillFile, path)
	}

	if err != nil {
		return fmt.Errorf("reading %s in %q: %w", skillFile, path, err)
	}

	name, description, body, err := parseFrontmatter(string(content))
	if err != nil {
		return fmt.Errorf("%w: %q", err, path)
	}

	if name == "" {
		return fmt.Errorf("%w: %q", ErrNameRequired, path)
	}

	if description == "" {
		return fmt.Errorf("%w: %q", ErrDescriptionRequired, path)
	}

	files, err := listFiles(root.FS())
	if err != nil {
		return fmt.Errorf("listing files in %q: %w", path, err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, taken := c.skills[name]; taken {
		return fmt.Errorf("%w: %q", ErrDuplicateSkill, name)
	}

	s := skill{
		name:        name,
		description: description,
		body:        body,
		skillPath:   path,
		files:       files,
	}
	c.skills[name] = s

	return nil
}

// Catalog renders the block of skill names and descriptions an agent adds to
// its system message, sorted by name so the prompt stays identical across
// sessions built from the same collection. It returns an empty string when no
// skills have been added.
func (c *Collection) Catalog() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.skills) == 0 {
		return ""
	}

	lines := []string{
		"<available_skills>",
	}

	for _, name := range slices.Sorted(maps.Keys(c.skills)) {
		s := c.skills[name]
		lines = append(lines,
			"  <skill>",
			"    <name>"+s.name+"</name>",
			"    <description>"+s.description+"</description>",
			"  </skill>",
		)
	}

	lines = append(lines, "</available_skills>")

	return strings.Join(lines, "\n")
}

// Skills returns the names of all skills in the collection, sorted by name.
func (c *Collection) Skills() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return slices.Sorted(maps.Keys(c.skills))
}

func (c *Collection) lookup(name string) (skill, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if s, found := c.skills[name]; found {
		return s, nil
	}

	available := slices.Sorted(maps.Keys(c.skills))

	return skill{}, fmt.Errorf("%w: %q; available skills: %s", ErrSkillNotFound, name, strings.Join(available, ", "))
}

func listFiles(fsys fs.FS) ([]string, error) {
	var files []string

	err := fs.WalkDir(fsys, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		info, err := fs.Stat(fsys, path)
		if err != nil || !info.Mode().IsRegular() || path == skillFile {
			return nil
		}

		files = append(files, path)

		return nil
	})
	if err != nil {
		return nil, err
	}

	slices.Sort(files)

	return files, nil
}

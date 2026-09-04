package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validSkill = "---\nname: git-release\ndescription: Draft release notes\n---\n\nDo the thing.\n"

const validSkillName = "git-release"

func writeSkill(t *testing.T, content string) string {
	t.Helper()

	path := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(content), 0o600))

	return path
}

func TestNewCollection(t *testing.T) {
	t.Run("returns an empty collection", func(t *testing.T) {
		// when
		result := NewCollection()
		// then
		require.NotNil(t, result)
		assert.Empty(t, result.Catalog())
	})
}

func TestCollectionAdd(t *testing.T) {
	t.Run("adds a skill the catalog then lists", func(t *testing.T) {
		// given
		collection := NewCollection()
		// when
		err := collection.Add(writeSkill(t, validSkill))
		// then
		require.NoError(t, err)
		assert.Contains(t, collection.Catalog(), "<name>git-release</name>")
		assert.Contains(t, collection.Catalog(), "<description>Draft release notes</description>")
	})

	t.Run("returns ErrSkillFolderNotFound when the path is not a folder", func(t *testing.T) {
		tests := map[string]func(t *testing.T) string{
			"missing folder": func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nope")
			},
			"path is a file": func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "SKILL.md")
				require.NoError(t, os.WriteFile(path, []byte(validSkill), 0o600))
				return path
			},
		}

		for name, fixture := range tests {
			t.Run(name, func(t *testing.T) {
				// given
				collection := NewCollection()
				// when
				err := collection.Add(fixture(t))
				// then
				assert.ErrorIs(t, err, ErrSkillFolderNotFound)
			})
		}
	})

	t.Run("returns ErrNoSkillFile when the folder holds no SKILL.md", func(t *testing.T) {
		// given
		collection := NewCollection()
		// when
		err := collection.Add(t.TempDir())
		// then
		assert.ErrorIs(t, err, ErrNoSkillFile)
	})

	t.Run("rejects a SKILL.md the parser cannot read", func(t *testing.T) {
		tests := map[string]struct {
			content  string
			expected error
		}{
			"malformed frontmatter": {content: "no fence here\n", expected: ErrInvalidFrontmatter},
			"missing name":          {content: "---\ndescription: does things\n---\nbody\n", expected: ErrNameRequired},
			"missing description":   {content: "---\nname: skill\n---\nbody\n", expected: ErrDescriptionRequired},
		}

		for name, tc := range tests {
			t.Run(name, func(t *testing.T) {
				// given
				collection := NewCollection()
				// when
				err := collection.Add(writeSkill(t, tc.content))
				// then
				assert.ErrorIs(t, err, tc.expected)
			})
		}
	})

	t.Run("returns ErrDuplicateSkill for a name already added", func(t *testing.T) {
		// given
		collection := NewCollection()
		require.NoError(t, collection.Add(writeSkill(t, validSkill)))
		// when
		err := collection.Add(writeSkill(t, validSkill))
		// then
		assert.ErrorIs(t, err, ErrDuplicateSkill)
	})
}

func TestCollectionAddClaudeSkill(t *testing.T) {
	writeClaudeSkill := func(t *testing.T, name, content string) {
		t.Helper()

		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)

		path := filepath.Join(home, ".claude", "skills", name)
		require.NoError(t, os.MkdirAll(path, 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(content), 0o600))
	}

	t.Run("adds a skill from the user's Claude skills folder", func(t *testing.T) {
		// given
		writeClaudeSkill(t, "git-release", validSkill)
		collection := NewCollection()
		// when
		err := collection.AddClaudeSkill("git-release")
		// then
		require.NoError(t, err)
		assert.Equal(t, []string{validSkillName}, collection.Skills())
	})

	t.Run("returns ErrSkillFolderNotFound when the folder holds no such skill", func(t *testing.T) {
		// given
		writeClaudeSkill(t, "git-release", validSkill)
		collection := NewCollection()
		// when
		err := collection.AddClaudeSkill("nope")
		// then
		assert.ErrorIs(t, err, ErrSkillFolderNotFound)
		assert.Empty(t, collection.Skills())
	})

	t.Run("returns ErrInvalidSkillName when the name is not a single folder", func(t *testing.T) {
		tests := map[string]string{
			"empty":            "",
			"parent":           "..",
			"escaping path":    "../../etc",
			"nested path":      "team/git-release",
			"trailing dot dot": "git-release/..",
		}

		for name, skillName := range tests {
			t.Run(name, func(t *testing.T) {
				// given
				writeClaudeSkill(t, "git-release", validSkill)
				collection := NewCollection()
				// when
				err := collection.AddClaudeSkill(skillName)
				// then
				assert.ErrorIs(t, err, ErrInvalidSkillName)
				assert.Empty(t, collection.Skills())
			})
		}
	})
}

func TestCollectionCatalog(t *testing.T) {
	t.Run("returns an empty string when no skills were added", func(t *testing.T) {
		// given
		collection := NewCollection()
		// when
		result := collection.Catalog()
		// then
		assert.Empty(t, result)
	})

	t.Run("renders the skills sorted by name", func(t *testing.T) {
		// given
		collection := NewCollection()
		require.NoError(t, collection.Add(writeSkill(t, "---\nname: beta\ndescription: second\n---\nb\n")))
		require.NoError(t, collection.Add(writeSkill(t, "---\nname: alpha\ndescription: first\n---\na\n")))
		// when
		result := collection.Catalog()
		// then
		expected := "<available_skills>\n" +
			"  <skill>\n" +
			"    <name>alpha</name>\n" +
			"    <description>first</description>\n" +
			"  </skill>\n" +
			"  <skill>\n" +
			"    <name>beta</name>\n" +
			"    <description>second</description>\n" +
			"  </skill>\n" +
			"</available_skills>"
		assert.Equal(t, expected, result)
	})
}

func TestCollectionSkills(t *testing.T) {
	t.Run("returns nothing when no skills were added", func(t *testing.T) {
		// given
		collection := NewCollection()
		// when
		result := collection.Skills()
		// then
		assert.Empty(t, result)
	})

	t.Run("returns the name of the only skill", func(t *testing.T) {
		// given
		collection := NewCollection()
		require.NoError(t, collection.Add(writeSkill(t, validSkill)))
		// when
		result := collection.Skills()
		// then
		expected := []string{"git-release"}
		assert.Equal(t, expected, result)
	})

	t.Run("returns the names sorted by name", func(t *testing.T) {
		// given
		collection := NewCollection()
		require.NoError(t, collection.Add(writeSkill(t, "---\nname: beta\ndescription: second\n---\nb\n")))
		require.NoError(t, collection.Add(writeSkill(t, "---\nname: alpha\ndescription: first\n---\na\n")))
		// when
		result := collection.Skills()
		// then
		expected := []string{"alpha", "beta"}
		assert.Equal(t, expected, result)
	})
}

func TestCollectionConcurrentUse(t *testing.T) {
	t.Run("adds and renders from several goroutines", func(t *testing.T) {
		// given
		collection := NewCollection()
		paths := make([]string, 8)
		for i := range paths {
			paths[i] = writeSkill(t, fmt.Sprintf("---\nname: skill-%d\ndescription: number %d\n---\nbody\n", i, i))
		}
		// when
		var group sync.WaitGroup
		for _, path := range paths {
			group.Add(2)

			go func() {
				defer group.Done()
				assert.NoError(t, collection.Add(path))
			}()

			go func() {
				defer group.Done()
				collection.Catalog()
			}()
		}
		group.Wait()
		// then
		result := collection.Catalog()
		for i := range paths {
			assert.Contains(t, result, fmt.Sprintf("<name>skill-%d</name>", i))
		}
	})
}

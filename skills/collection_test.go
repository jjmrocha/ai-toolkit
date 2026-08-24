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
		expected := "Skills provide specialized instructions and workflows for specific tasks.\n" +
			"Use the skill_load tool to load a skill when a task matches its description.\n" +
			"\n" +
			"<available_skills>\n" +
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

package skills

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFrontmatter(t *testing.T) {
	t.Run("reads the name and description", func(t *testing.T) {
		// given
		content := "---\nname: git-release\ndescription: Draft release notes\n---\n\nDo the thing.\n"
		// when
		name, description, body, err := parseFrontmatter(content)
		// then
		require.NoError(t, err)
		assert.Equal(t, "git-release", name)
		assert.Equal(t, "Draft release notes", description)
		assert.Equal(t, "Do the thing.\n", body)
	})

	t.Run("keeps colons inside a quoted value", func(t *testing.T) {
		// given
		content := "---\nname: review\ndescription: \"Use when auditing: diffs, PRs — or a branch\"\n---\nbody\n"
		// when
		_, result, _, err := parseFrontmatter(content)
		// then
		require.NoError(t, err)
		expected := "Use when auditing: diffs, PRs — or a branch"
		assert.Equal(t, expected, result)
	})

	t.Run("strips surrounding quotes", func(t *testing.T) {
		tests := map[string]struct {
			line     string
			expected string
		}{
			"double quotes": {line: `description: "Triggers on 'build X', or similar"`, expected: "Triggers on 'build X', or similar"},
			"single quotes": {line: `description: 'a quoted value'`, expected: "a quoted value"},
		}

		for name, tc := range tests {
			t.Run(name, func(t *testing.T) {
				// given
				content := "---\nname: skill\n" + tc.line + "\n---\nbody\n"
				// when
				_, result, _, err := parseFrontmatter(content)
				// then
				require.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			})
		}
	})

	t.Run("reads block scalars", func(t *testing.T) {
		tests := map[string]struct {
			content  string
			expected string
		}{
			"folded":          {content: "---\nname: skill\ndescription: >-\n  does things\n  and more\n---\nbody\n", expected: "does things and more"},
			"literal":         {content: "---\nname: skill\ndescription: |-\n  does things\n---\nbody\n", expected: "does things"},
			"plain multiline": {content: "---\nname: skill\ndescription: does things\n  and more\n---\nbody\n", expected: "does things and more"},
		}

		for name, tc := range tests {
			t.Run(name, func(t *testing.T) {
				// when
				_, result, _, err := parseFrontmatter(tc.content)
				// then
				require.NoError(t, err)
				assert.Equal(t, tc.expected, result)
			})
		}
	})

	t.Run("ignores unknown keys", func(t *testing.T) {
		tests := map[string]string{
			"scalar values":  "---\nname: skill\nlicense: MIT\ndescription: does things\ncompatibility: opencode\n---\nbody\n",
			"nested mapping": "---\nname: skill\ndescription: does things\nmetadata:\n  author: someone\n  version: 2\n---\nbody\n",
			"sequence":       "---\nname: skill\ndescription: does things\nallowed-tools:\n  - Read\n  - Bash\n---\nbody\n",
		}

		for name, content := range tests {
			t.Run(name, func(t *testing.T) {
				// when
				name, description, _, err := parseFrontmatter(content)
				// then
				require.NoError(t, err)
				assert.Equal(t, "skill", name)
				assert.Equal(t, "does things", description)
			})
		}
	})

	t.Run("skips blank lines inside the frontmatter", func(t *testing.T) {
		// given
		content := "---\nname: skill\n\ndescription: does things\n---\nbody\n"
		// when
		name, description, _, err := parseFrontmatter(content)
		// then
		require.NoError(t, err)
		assert.Equal(t, "skill", name)
		assert.Equal(t, "does things", description)
	})

	t.Run("trims leading blank lines from the body", func(t *testing.T) {
		// given
		content := "---\nname: skill\ndescription: does things\n---\n\n\n# Heading\n\ntext\n"
		// when
		_, _, result, err := parseFrontmatter(content)
		// then
		require.NoError(t, err)
		expected := "# Heading\n\ntext\n"
		assert.Equal(t, expected, result)
	})

	t.Run("reads a file with carriage returns", func(t *testing.T) {
		// given
		content := "---\r\nname: skill\r\ndescription: does things\r\n---\r\nbody\r\n"
		// when
		name, description, _, err := parseFrontmatter(content)
		// then
		require.NoError(t, err)
		assert.Equal(t, "skill", name)
		assert.Equal(t, "does things", description)
	})

	t.Run("returns an empty value when a key carries none", func(t *testing.T) {
		tests := map[string]string{
			"key absent": "---\nname: skill\n---\nbody\n",
			"key empty":  "---\nname: skill\ndescription:\n---\nbody\n",
		}

		for name, content := range tests {
			t.Run(name, func(t *testing.T) {
				// when
				name, description, _, err := parseFrontmatter(content)
				// then
				require.NoError(t, err)
				assert.Equal(t, "skill", name)
				assert.Empty(t, description)
			})
		}
	})

	t.Run("returns ErrInvalidFrontmatter for malformed content", func(t *testing.T) {
		tests := map[string]string{
			"empty content":            "",
			"no opening fence":         "name: skill\ndescription: does things\n---\nbody\n",
			"no closing fence":         "---\nname: skill\ndescription: does things\nbody\n",
			"line without a colon":     "---\nname: skill\nnonsense\ndescription: does things\n---\nbody\n",
			"unquoted colon in value":  "---\nname: skill\ndescription: Use when auditing: diffs\n---\nbody\n",
			"duplicate key":            "---\nname: skill\nname: other\ndescription: does things\n---\nbody\n",
			"not a mapping":            "---\njust a string\n---\nbody\n",
			"non-string value":         "---\nname: skill\ndescription:\n  - does things\n---\nbody\n",
			"body text before a fence": "not frontmatter\n---\nname: skill\n---\nbody\n",
		}

		for name, content := range tests {
			t.Run(name, func(t *testing.T) {
				// when
				_, _, _, err := parseFrontmatter(content)
				// then
				assert.ErrorIs(t, err, ErrInvalidFrontmatter)
			})
		}
	})
}

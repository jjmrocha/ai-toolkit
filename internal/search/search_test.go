package search

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dirWith(t *testing.T, files map[string]string) string {
	t.Helper()

	dir := t.TempDir()

	for name, content := range files {
		full := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o700))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o600))
	}

	return dir
}

func TestFiles(t *testing.T) {
	t.Run("returns the matching lines with their path and line number", func(t *testing.T) {
		// given
		dir := dirWith(t, map[string]string{"notes.md": "alpha\nbeta\ngamma beta\n"})
		cfg := Config{Dir: dir, Pattern: regexp.MustCompile("beta")}
		// when
		result, err := Files(t.Context(), cfg)
		// then
		require.NoError(t, err)
		expected := []Match{
			{Path: filepath.Join(dir, "notes.md"), Line: 2, Text: "beta"},
			{Path: filepath.Join(dir, "notes.md"), Line: 3, Text: "gamma beta"},
		}
		assert.Equal(t, expected, result.Matches)
		assert.False(t, result.Truncated)
	})

	t.Run("cuts a line longer than MaxTextBytes and marks the match truncated", func(t *testing.T) {
		// given
		dir := dirWith(t, map[string]string{"notes.md": "beta " + strings.Repeat("x", 100) + "\n"})
		cfg := Config{Dir: dir, Pattern: regexp.MustCompile("beta"), MaxTextBytes: 10}
		// when
		result, err := Files(t.Context(), cfg)
		// then
		require.NoError(t, err)
		expected := []Match{
			{Path: filepath.Join(dir, "notes.md"), Line: 1, Text: "beta xxxxx", Truncated: true},
		}
		assert.Equal(t, expected, result.Matches)
	})

	t.Run("keeps whole lines when MaxTextBytes is zero", func(t *testing.T) {
		// given
		long := "beta " + strings.Repeat("x", 100)
		dir := dirWith(t, map[string]string{"notes.md": long + "\n"})
		cfg := Config{Dir: dir, Pattern: regexp.MustCompile("beta")}
		// when
		result, err := Files(t.Context(), cfg)
		// then
		require.NoError(t, err)
		expected := []Match{
			{Path: filepath.Join(dir, "notes.md"), Line: 1, Text: long},
		}
		assert.Equal(t, expected, result.Matches)
	})

	t.Run("cuts at a whole character", func(t *testing.T) {
		// given
		dir := dirWith(t, map[string]string{"notes.md": "beta \u00e9" + strings.Repeat("x", 20) + "\n"})
		cfg := Config{Dir: dir, Pattern: regexp.MustCompile("beta"), MaxTextBytes: 6}
		// when
		result, err := Files(t.Context(), cfg)
		// then
		require.NoError(t, err)
		require.Len(t, result.Matches, 1)
		expected := "beta "
		assert.Equal(t, expected, result.Matches[0].Text)
		assert.True(t, utf8.ValidString(result.Matches[0].Text))
	})

	t.Run("returns no matches when nothing matches", func(t *testing.T) {
		// given
		dir := dirWith(t, map[string]string{"notes.md": "alpha\n"})
		cfg := Config{Dir: dir, Pattern: regexp.MustCompile("beta")}
		// when
		result, err := Files(t.Context(), cfg)
		// then
		require.NoError(t, err)
		assert.Empty(t, result.Matches)
		assert.False(t, result.Truncated)
	})

	t.Run("matches the last line of a file without a trailing newline", func(t *testing.T) {
		// given
		dir := dirWith(t, map[string]string{"notes.md": "alpha\nbeta"})
		cfg := Config{Dir: dir, Pattern: regexp.MustCompile("beta")}
		// when
		result, err := Files(t.Context(), cfg)
		// then
		require.NoError(t, err)
		expected := []Match{{Path: filepath.Join(dir, "notes.md"), Line: 2, Text: "beta"}}
		assert.Equal(t, expected, result.Matches)
	})

	t.Run("walks the files in lexical order", func(t *testing.T) {
		// given
		dir := dirWith(t, map[string]string{"b.md": "beta\n", "a.md": "beta\n", "c.md": "beta\n"})
		cfg := Config{Dir: dir, Pattern: regexp.MustCompile("beta")}
		// when
		result, err := Files(t.Context(), cfg)
		// then
		require.NoError(t, err)
		expected := []string{
			filepath.Join(dir, "a.md"),
			filepath.Join(dir, "b.md"),
			filepath.Join(dir, "c.md"),
		}
		var paths []string
		for _, match := range result.Matches {
			paths = append(paths, match.Path)
		}

		assert.Equal(t, expected, paths)
	})

	t.Run("searches the folders below Dir when Recursive is set", func(t *testing.T) {
		// given
		dir := dirWith(t, map[string]string{
			"top.md":           "beta\n",
			"deep/down/low.md": "beta\n",
		})
		cfg := Config{Dir: dir, Pattern: regexp.MustCompile("beta"), Recursive: true}
		// when
		result, err := Files(t.Context(), cfg)
		// then
		require.NoError(t, err)
		expected := []Match{
			{Path: filepath.Join(dir, "deep/down/low.md"), Line: 1, Text: "beta"},
			{Path: filepath.Join(dir, "top.md"), Line: 1, Text: "beta"},
		}
		assert.Equal(t, expected, result.Matches)
	})

	t.Run("searches only Dir when Recursive is not set", func(t *testing.T) {
		// given
		dir := dirWith(t, map[string]string{
			"top.md":           "beta\n",
			"deep/down/low.md": "beta\n",
		})
		cfg := Config{Dir: dir, Pattern: regexp.MustCompile("beta")}
		// when
		result, err := Files(t.Context(), cfg)
		// then
		require.NoError(t, err)
		expected := []Match{{Path: filepath.Join(dir, "top.md"), Line: 1, Text: "beta"}}
		assert.Equal(t, expected, result.Matches)
	})

	t.Run("searches every file when Glob is empty", func(t *testing.T) {
		// given
		dir := dirWith(t, map[string]string{"notes.md": "beta\n", "data.csv": "beta\n"})
		cfg := Config{Dir: dir, Pattern: regexp.MustCompile("beta")}
		// when
		result, err := Files(t.Context(), cfg)
		// then
		require.NoError(t, err)
		assert.Len(t, result.Matches, 2)
	})

	t.Run("matches Glob against the file name at any depth", func(t *testing.T) {
		// given
		dir := dirWith(t, map[string]string{
			"data.csv":        "beta\n",
			"deep/notes.md":   "beta\n",
			"deep/report.csv": "beta\n",
		})
		cfg := Config{
			Dir:       dir,
			Pattern:   regexp.MustCompile("beta"),
			Glob:      "*.md",
			Recursive: true,
		}
		// when
		result, err := Files(t.Context(), cfg)
		// then
		require.NoError(t, err)
		expected := []Match{{Path: filepath.Join(dir, "deep/notes.md"), Line: 1, Text: "beta"}}
		assert.Equal(t, expected, result.Matches)
	})

	t.Run("collects everything when MaxMatches is not set", func(t *testing.T) {
		// given
		dir := dirWith(t, map[string]string{"notes.md": strings.Repeat("beta\n", 50)})
		cfg := Config{Dir: dir, Pattern: regexp.MustCompile("beta")}
		// when
		result, err := Files(t.Context(), cfg)
		// then
		require.NoError(t, err)
		assert.Len(t, result.Matches, 50)
		assert.False(t, result.Truncated)
	})

	t.Run("stops on MaxMatches and marks the result truncated", func(t *testing.T) {
		// given
		dir := dirWith(t, map[string]string{"a.md": "beta\nbeta\n", "b.md": "beta\nbeta\n"})
		cfg := Config{Dir: dir, Pattern: regexp.MustCompile("beta"), MaxMatches: 3}
		// when
		result, err := Files(t.Context(), cfg)
		// then
		require.NoError(t, err)
		expected := []Match{
			{Path: filepath.Join(dir, "a.md"), Line: 1, Text: "beta"},
			{Path: filepath.Join(dir, "a.md"), Line: 2, Text: "beta"},
			{Path: filepath.Join(dir, "b.md"), Line: 1, Text: "beta"},
		}
		assert.Equal(t, expected, result.Matches)
		assert.True(t, result.Truncated)
	})

	t.Run("is not truncated when the matches fit MaxMatches exactly", func(t *testing.T) {
		// given
		dir := dirWith(t, map[string]string{"a.md": "beta\nbeta\n", "b.md": "beta\n"})
		cfg := Config{Dir: dir, Pattern: regexp.MustCompile("beta"), MaxMatches: 3}
		// when
		result, err := Files(t.Context(), cfg)
		// then
		require.NoError(t, err)
		assert.Len(t, result.Matches, 3)
		assert.False(t, result.Truncated)
	})

	t.Run("skips a file holding a NUL byte", func(t *testing.T) {
		// given
		dir := dirWith(t, map[string]string{"a.bin": "beta\x00beta\n", "b.md": "beta\n"})
		cfg := Config{Dir: dir, Pattern: regexp.MustCompile("beta")}
		// when
		result, err := Files(t.Context(), cfg)
		// then
		require.NoError(t, err)
		expected := []Match{{Path: filepath.Join(dir, "b.md"), Line: 1, Text: "beta"}}
		assert.Equal(t, expected, result.Matches)
	})

	t.Run("reads a file whose first 8 KiB hold no NUL byte", func(t *testing.T) {
		// given
		content := "beta\n" + strings.Repeat("x", 9000) + "\n\x00\n"
		dir := dirWith(t, map[string]string{"late.md": content})
		cfg := Config{Dir: dir, Pattern: regexp.MustCompile("beta")}
		// when
		result, err := Files(t.Context(), cfg)
		// then
		require.NoError(t, err)
		expected := []Match{{Path: filepath.Join(dir, "late.md"), Line: 1, Text: "beta"}}
		assert.Equal(t, expected, result.Matches)
	})

	t.Run("skips a symbolic link that leaves the folder", func(t *testing.T) {
		// given
		dir := dirWith(t, map[string]string{"inside.md": "beta\n"})
		outside := filepath.Join(t.TempDir(), "outside.md")
		require.NoError(t, os.WriteFile(outside, []byte("beta\n"), 0o600))
		require.NoError(t, os.Symlink(outside, filepath.Join(dir, "link.md")))

		cfg := Config{Dir: dir, Pattern: regexp.MustCompile("beta")}
		// when
		result, err := Files(t.Context(), cfg)
		// then
		require.NoError(t, err)
		expected := []Match{{Path: filepath.Join(dir, "inside.md"), Line: 1, Text: "beta"}}
		assert.Equal(t, expected, result.Matches)
	})

	t.Run("skips a symbolic link inside the folder", func(t *testing.T) {
		// given
		dir := dirWith(t, map[string]string{"target.md": "beta\n"})
		require.NoError(t, os.Symlink("target.md", filepath.Join(dir, "alias.md")))

		cfg := Config{Dir: dir, Pattern: regexp.MustCompile("beta")}
		// when
		result, err := Files(t.Context(), cfg)
		// then
		require.NoError(t, err)
		expected := []Match{{Path: filepath.Join(dir, "target.md"), Line: 1, Text: "beta"}}
		assert.Equal(t, expected, result.Matches)
	})

	t.Run("discards the matches of a file holding a line over 1 MiB", func(t *testing.T) {
		// given
		dir := dirWith(t, map[string]string{
			"a.md": "beta\n" + strings.Repeat("x", 2*1024*1024) + "\n",
			"b.md": "beta\n",
		})
		cfg := Config{Dir: dir, Pattern: regexp.MustCompile("beta")}
		// when
		result, err := Files(t.Context(), cfg)
		// then
		require.NoError(t, err)
		expected := []Match{{Path: filepath.Join(dir, "b.md"), Line: 1, Text: "beta"}}
		assert.Equal(t, expected, result.Matches)
	})

	t.Run("returns an error when Dir does not exist", func(t *testing.T) {
		// given
		cfg := Config{
			Dir:     filepath.Join(t.TempDir(), "missing"),
			Pattern: regexp.MustCompile("beta"),
		}
		// when
		result, err := Files(t.Context(), cfg)
		// then
		assert.ErrorIs(t, err, os.ErrNotExist)
		assert.Nil(t, result)
	})

	t.Run("stops when the context is done", func(t *testing.T) {
		// given
		dir := dirWith(t, map[string]string{"notes.md": "beta\n"})
		cfg := Config{Dir: dir, Pattern: regexp.MustCompile("beta")}

		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		// when
		result, err := Files(ctx, cfg)
		// then
		assert.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, result)
	})
}

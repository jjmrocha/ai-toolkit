package helper

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"unicode/utf8"
)

const (
	binarySniffBytes  = 8 * 1024
	maxSearchLineSize = 1024 * 1024
)

// SearchConfig describes the search [Search] runs over a folder.
type SearchConfig struct {
	// Dir is the folder to search, as an absolute path. Search walks it in
	// lexical order, so the same search twice returns the same matches.
	Dir string
	// Pattern is the expression every line is matched against. The caller
	// compiles it, so a pattern the model got wrong is refused before anything
	// is read.
	Pattern *regexp.Regexp
	// Glob is matched against a file's name alone, never its path, so "*.md"
	// finds a file called that however deep it sits. An empty Glob searches
	// every file.
	Glob string
	// Recursive reports whether the folders under [SearchConfig.Dir] are
	// searched too. A false Recursive searches only the files directly in it.
	Recursive bool
	// MaxMatches is how many matches Search collects before it stops walking,
	// which is what bounds the work a broad pattern costs. A zero MaxMatches
	// collects everything.
	MaxMatches int
	// MaxTextBytes is how much of a matching line [SearchMatch.Text] carries. A
	// longer line is cut at the last whole character that fits, so a match never
	// holds more than this however long the line was, and
	// [SearchMatch.Truncated] says it was cut. A zero MaxTextBytes keeps whole
	// lines.
	MaxTextBytes int
}

// SearchMatch is one line that matched [SearchConfig.Pattern].
type SearchMatch struct {
	// Path is the file the line came from, as an absolute path.
	Path string
	// Line is the line's number in that file, counting from 1.
	Line int
	// Text is the line itself, with the newline removed, cut to
	// [SearchConfig.MaxTextBytes] when it ran over.
	Text string
	// Truncated reports whether the line was cut to
	// [SearchConfig.MaxTextBytes], so Text holds only its start.
	Truncated bool
}

// SearchResult is what [Search] collected before it stopped.
type SearchResult struct {
	// Matches holds the matching lines, in the order the walk found them:
	// grouped by file, and by line number within a file.
	Matches []SearchMatch
	// Truncated reports whether the walk stopped on [SearchConfig.MaxMatches]
	// with matches still to find. Search looks one match past the limit to tell
	// a file that ended exactly on it from one that did not, so a false
	// Truncated means [SearchResult.Matches] holds everything there was.
	Truncated bool
}

// Search reads every file under [SearchConfig.Dir] that [SearchConfig.Glob]
// names and returns the lines matching [SearchConfig.Pattern], stopping once it
// has [SearchConfig.MaxMatches] of them.
//
// The folder is opened as an [os.Root] and walked from inside it, so a symbolic
// link that leaves the folder is refused rather than followed, and a caller
// that already confined the path keeps that confinement here.
//
// Three kinds of file are passed over in silence: one that is not a regular
// file; one holding a NUL byte in its first 8 KiB, the test grep uses for a
// binary; and one that fails to open or read, so a single unreadable file does
// not void a search that found matches elsewhere. A line longer than 1 MiB makes
// a file unreadable, so its matches are discarded along with it.
//
// Search returns an error only when [SearchConfig.Dir] itself cannot be opened
// or walked, or when ctx ends first.
func Search(ctx context.Context, cfg SearchConfig) (*SearchResult, error) {
	root, err := os.OpenRoot(cfg.Dir)
	if err != nil {
		return nil, err
	}

	defer func() { _ = root.Close() }()

	limit := 0
	if cfg.MaxMatches > 0 {
		limit = cfg.MaxMatches + 1
	}

	var result SearchResult

	err = fs.WalkDir(root.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == "." {
				return walkErr
			}

			return nil
		}

		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		if entry.IsDir() {
			if cfg.Recursive || path == "." {
				return nil
			}

			return fs.SkipDir
		}

		if !entry.Type().IsRegular() || !matchesGlob(cfg.Glob, entry.Name()) {
			return nil
		}

		matches := searchFile(root, path, cfg.Pattern, cfg.MaxTextBytes, limit-len(result.Matches))
		result.Matches = append(result.Matches, matches...)

		if limit > 0 && len(result.Matches) == limit {
			return fs.SkipAll
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	if limit > 0 && len(result.Matches) == limit {
		result.Matches = result.Matches[:cfg.MaxMatches]
		result.Truncated = true
	}

	return &result, nil
}

func elide(text string, maxText int) (string, bool) {
	if maxText == 0 || len(text) <= maxText {
		return text, false
	}

	cut := maxText
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}

	return text[:cut], true
}

func matchesGlob(glob string, name string) bool {
	if glob == "" {
		return true
	}

	matched, err := filepath.Match(glob, name)

	return err == nil && matched
}

func searchFile(root *os.Root, name string, pattern *regexp.Regexp, maxText int, budget int) []SearchMatch {
	file, err := root.Open(name)
	if err != nil {
		return nil
	}

	defer func() { _ = file.Close() }()

	reader := bufio.NewReaderSize(file, binarySniffBytes)

	head, err := reader.Peek(binarySniffBytes)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil
	}

	if bytes.IndexByte(head, 0) >= 0 {
		return nil
	}

	full := filepath.Join(root.Name(), name)

	var matches []SearchMatch

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(nil, maxSearchLineSize)

	for line := 1; scanner.Scan(); line++ {
		if !pattern.Match(scanner.Bytes()) {
			continue
		}

		text, truncated := elide(scanner.Text(), maxText)

		match := SearchMatch{
			Path:      full,
			Line:      line,
			Text:      text,
			Truncated: truncated,
		}
		matches = append(matches, match)

		if budget > 0 && len(matches) == budget {
			return matches
		}
	}

	if scanner.Err() != nil {
		return nil
	}

	return matches
}

package search

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

// Config describes a search [Files] runs.
type Config struct {
	// Dir is the folder to search, as an absolute path. Files walks it in lexical
	// order, so the same search returns the same matches.
	Dir string
	// Pattern is matched against every line. The caller compiles it, so a bad
	// pattern is refused before anything is read.
	Pattern *regexp.Regexp
	// Glob is matched against a file's name, not its path, so "*.md" finds such a
	// file at any depth. Empty searches every file.
	Glob string
	// Recursive reports whether folders under [Config.Dir] are searched too. When
	// false, only the files directly in it are.
	Recursive bool
	// MaxMatches is how many matches Files collects before it stops walking, which
	// bounds the cost of a broad pattern. Zero collects everything.
	MaxMatches int
	// MaxTextBytes caps the bytes of a line kept in [Match.Text]. A longer line is
	// cut at the last whole character that fits, and [Match.Truncated] is set.
	// Zero keeps whole lines.
	MaxTextBytes int
}

// Match is one line that matched [Config.Pattern].
type Match struct {
	// Path is the file's absolute path.
	Path string
	// Line is the line's number in that file, counting from 1.
	Line int
	// Text is the line without its newline, cut to [Config.MaxTextBytes] if
	// longer.
	Text string
	// Truncated reports whether Text was cut to [Config.MaxTextBytes].
	Truncated bool
}

// Result is what [Files] collected before it stopped.
type Result struct {
	// Matches are the matching lines in the order found: grouped by file, and by
	// line number within a file.
	Matches []Match
	// Truncated reports whether the walk stopped at [Config.MaxMatches] with more
	// matches left. Files looks one match past the limit, so a false Truncated
	// means Matches holds every match.
	Truncated bool
}

// Files reads every file under [Config.Dir] that [Config.Glob] matches and
// returns the lines matching [Config.Pattern], stopping at [Config.MaxMatches].
//
// The folder is opened as an [os.Root] and walked from inside, so a symbolic
// link that leaves it is refused, and a path the caller already confined stays
// confined.
//
// Three kinds of file are skipped silently: anything not a regular file; a file
// with a NUL byte in its first 8 KiB, grep's test for binary; and a file that
// fails to open or read, so one bad file does not spoil the search. A line over
// 1 MiB makes a file unreadable, so its matches are dropped.
//
// Files returns an error only when [Config.Dir] cannot be opened or walked,
// when [Config.Glob] is malformed, or when ctx ends first. A malformed Glob is
// reported with [filepath.ErrBadPattern] once a file name reaches the part that
// does not parse.
func Files(ctx context.Context, cfg Config) (*Result, error) {
	root, err := os.OpenRoot(cfg.Dir)
	if err != nil {
		return nil, err
	}

	defer func() { _ = root.Close() }()

	limit := 0
	if cfg.MaxMatches > 0 {
		limit = cfg.MaxMatches + 1
	}

	var result Result

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

		matched, globErr := matchesGlob(cfg.Glob, entry.Name())
		if globErr != nil {
			return globErr
		}

		if !entry.Type().IsRegular() || !matched {
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

func matchesGlob(glob string, name string) (bool, error) {
	if glob == "" {
		return true, nil
	}

	return filepath.Match(glob, name)
}

func searchFile(root *os.Root, name string, pattern *regexp.Regexp, maxText int, budget int) []Match {
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

	var matches []Match

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(nil, maxSearchLineSize)

	for line := 1; scanner.Scan(); line++ {
		if !pattern.Match(scanner.Bytes()) {
			continue
		}

		text, truncated := elide(scanner.Text(), maxText)

		match := Match{
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

package packs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/jjmrocha/ai-toolkit/internal/search"
	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/mcp"
	"github.com/jjmrocha/ai-toolkit/tools"
)

const (
	readToolName      = "file_read"
	writeToolName     = "file_write"
	editToolName      = "file_edit"
	listToolName      = "file_list"
	searchToolName    = "file_search"
	deleteToolName    = "file_delete"
	workdirToolName   = "file_workdir"
	pathArg           = "path"
	contentArg        = "content"
	oldStringArg      = "old_string"
	newStringArg      = "new_string"
	offsetArg         = "offset"
	limitArg          = "limit"
	patternArg        = "pattern"
	globArg           = "glob"
	recursiveArg      = "recursive"
	defaultReadLines  = 2000
	maxSearchMatches  = 100
	maxMatchTextBytes = 500
	maxFileReadBytes  = 1024 * 1024
)

const fileInstruction = `These tools reach only the folder %s and nothing
outside it; name every file relative to that folder and never absolute. Find
which file says something with file_search rather than reading the folder
through, and when a search ends exactly on its limit it reported the whole
answer. file_edit changes one chosen place: include enough of the surroundings
to make the target text unambiguous. Never delete work you did not make, and
never discard a whole folder's content behind a single call.`

type filePack struct {
	toolBox *tools.ToolBox
	root    *os.Root
	path    string
	names   []string
	once    sync.Once
}

func (p *filePack) Instructions(_ context.Context) *mcp.Instruction {
	return &mcp.Instruction{
		Name: "file",
		Text: fmt.Sprintf(fileInstruction, p.path),
	}
}

func (p *filePack) Close() error {
	var err error

	p.once.Do(func() {
		for _, name := range p.names {
			p.toolBox.Remove(name)
		}

		err = p.root.Close()
	})

	return err
}

// FileTools registers seven tools in m that work on the files under path and
// nowhere else:
//
//   - "file_read" reads a text file one page of lines at a time
//   - "file_write" writes a whole file
//   - "file_edit" replaces one piece of text in a file
//   - "file_list" lists one folder
//   - "file_search" finds the lines that match a regular expression
//   - "file_delete" removes a file or an empty folder
//   - "file_workdir" returns the folder's absolute path, for naming a file to a
//     tool outside the pack
//
// [ToolPack.Close] removes the tools and closes the folder. FileTools returns an
// error, and registers nothing, if path cannot be opened.
//
// The confinement is [os.Root]: a path that climbs out, is absolute, or leaves
// through a symbolic link is refused. That is what separates this pack from
// [CodingTools] and [ShellTools]. Inside the folder, the tools have the
// program's authority. The folder's absolute path is not a secret:
// "file_workdir", "file_write", "file_list" and error messages all show it, so
// use a folder whose path is safe to disclose.
//
// "file_edit" writes nothing unless the text appears exactly once. "file_delete"
// refuses a folder that is not empty, so no single call deletes recursively.
// "file_search" returns paths relative to the folder, ready for "file_read",
// skips binary files, and returns at most 100 matches unless the call asks for
// more, marking a result it cut short.
func FileTools(m *tools.ToolBox, path string) (ToolPack, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("opening root: %w", err)
	}

	opened, err := os.OpenRoot(absPath)
	if err != nil {
		return nil, fmt.Errorf("opening root: %w", err)
	}

	pack := filePack{
		toolBox: m,
		root:    opened,
		path:    absPath,
	}

	readTool := llm.Tool{
		Name: readToolName,
		Description: "Read a text file, one page at a time. The result carries the lines returned and how " +
			"many the file holds, so a file longer than one page is read by calling again with a larger " +
			"offset.",
		Schema: tools.NewObjectBuilder().
			String(pathArg, "Path to the file, relative to the folder the tools are confined to", true).
			Integer(offsetArg, "First line to return, counting from 1", false).
			Integer(limitArg, "How many lines to return, defaulting to "+
				strconv.Itoa(defaultReadLines), false).
			Build(),
	}
	writeTool := llm.Tool{
		Name: writeToolName,
		Description: "Write a text file, replacing whatever it held. Folders the path needs are created. " +
			"Use it for a new file or a rewrite. The result names the file twice, as the path given here " +
			"and as its full path, which is the one to hand to a tool that works outside this folder.",
		Schema: tools.NewObjectBuilder().
			String(pathArg, "Path to the file, relative to the folder the tools are confined to", true).
			String(contentArg, "The full content of the file", true).
			Build(),
	}
	editTool := llm.Tool{
		Name: editToolName,
		Description: "Change part of a text file by replacing one piece of text with another. The text to " +
			"replace must appear exactly once in the file, so include enough of its surroundings to be " +
			"sure; nothing is written when it appears zero times or more than once. An empty replacement " +
			"removes the text.",
		Schema: tools.NewObjectBuilder().
			String(pathArg, "Path to the file, relative to the folder the tools are confined to", true).
			String(oldStringArg, "The exact text to replace, appearing once in the file", true).
			String(newStringArg, "The text to put in its place", true).
			Build(),
	}
	listTool := llm.Tool{
		Name: listToolName,
		Description: "List what a folder holds, sorted by name: one element per entry, carrying its name, " +
			"its size in bytes for a file, and its full path, which is the one to hand to a tool that " +
			"works outside this folder. It lists one folder, not the tree below it.",
		Schema: tools.NewObjectBuilder().
			String(pathArg, "Path to the folder, relative to the folder the tools are confined to, "+
				"defaulting to that folder itself", false).
			Build(),
	}
	searchTool := llm.Tool{
		Name: searchToolName,
		Description: "Search the files of a folder for the lines matching a regular expression, and " +
			"return each one with the file it came from and its line number. Use it to find which file " +
			"says something when the folder is too large to read through. Binary files are passed over. " +
			"The result says whether it was cut short, which is answered by narrowing the search or " +
			"asking for a larger " + limitArg + ".",
		Schema: tools.NewObjectBuilder().
			String(patternArg, "Regular expression in Go syntax, matched against one line at a time. "+
				"Prefix it with (?i) to ignore case", true).
			String(pathArg, "Path to the folder to search, relative to the folder the tools are "+
				"confined to, defaulting to that folder itself", false).
			String(globArg, "Pattern the file name must match for the file to be searched, such as "+
				"*.md. It is matched against the name alone, so it finds a file at any depth", false).
			Boolean(recursiveArg, "Whether to search the folders below as well, true by default", false).
			Integer(limitArg, "How many matches to return, defaulting to "+
				strconv.Itoa(maxSearchMatches), false).
			Build(),
	}
	deleteTool := llm.Tool{
		Name: deleteToolName,
		Description: "Delete a file, or a folder that is already empty. A folder that still holds anything " +
			"is kept, so its contents are deleted one by one or not at all. What is deleted does not come " +
			"back.",
		Schema: tools.NewObjectBuilder().
			String(pathArg, "Path to the file or empty folder, relative to the folder the tools are "+
				"confined to", true).
			Build(),
	}
	workdirTool := llm.Tool{
		Name: workdirToolName,
		Description: "Return the absolute path of the folder these tools work in. The file tools take " +
			"paths relative to it, so use this to build the full path of a file for a tool that works " +
			"outside this folder.",
		Schema: tools.NewObjectBuilder().Build(),
	}
	pack.names, err = register(m, []registration{
		{tool: readTool, handler: pack.readFile},
		{tool: writeTool, handler: pack.writeFile},
		{tool: editTool, handler: pack.editFile},
		{tool: listTool, handler: pack.listDir},
		{tool: searchTool, handler: pack.searchFiles},
		{tool: deleteTool, handler: pack.deleteFile},
		{tool: workdirTool, handler: pack.workdir},
	})
	if err != nil {
		return nil, err
	}

	return &pack, nil
}

func (p *filePack) workdir(_ context.Context, _ map[string]any) (string, error) {
	return p.path, nil
}

func (p *filePack) fullPath(entry string) string {
	return filepath.Join(p.path, entry)
}

func (p *filePack) readFile(_ context.Context, args map[string]any) (string, error) {
	arguments := tools.NewArguments(args)

	path, err := arguments.GetString(pathArg)
	if err != nil {
		return "", err
	}

	offset, err := positiveIntArg(arguments, offsetArg, 1)
	if err != nil {
		return "", err
	}

	limit, err := positiveIntArg(arguments, limitArg, defaultReadLines)
	if err != nil {
		return "", err
	}

	file, err := p.root.Open(path)
	if err != nil {
		return "", fmt.Errorf("reading %q: %w", path, err)
	}

	defer func() { _ = file.Close() }()

	lines, total, err := readPage(file, offset, limit)
	if err != nil {
		return "", fmt.Errorf("reading %q: %w", path, err)
	}

	return renderPage(lines, offset, total), nil
}

func (p *filePack) writeFile(_ context.Context, args map[string]any) (string, error) {
	arguments := tools.NewArguments(args)

	path, err := arguments.GetString(pathArg)
	if err != nil {
		return "", err
	}

	content, err := arguments.GetString(contentArg)
	if err != nil {
		return "", err
	}

	if dir := filepath.Dir(path); dir != "." {
		if err = p.root.MkdirAll(dir, 0o700); err != nil {
			return "", fmt.Errorf("writing %q: %w", path, err)
		}
	}

	if err = p.root.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("writing %q: %w", path, err)
	}

	return "wrote " + strconv.Itoa(len(content)) + " bytes to " + path + " - " + p.fullPath(path), nil
}

func (p *filePack) editFile(_ context.Context, args map[string]any) (string, error) {
	arguments := tools.NewArguments(args)

	path, err := arguments.GetString(pathArg)
	if err != nil {
		return "", err
	}

	oldString, err := arguments.GetString(oldStringArg)
	if err != nil {
		return "", err
	}

	newString, err := arguments.GetString(newStringArg)
	if err != nil {
		return "", err
	}

	content, err := p.root.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("editing %q: %w", path, err)
	}

	switch strings.Count(string(content), oldString) {
	case 1:
	case 0:
		return "", fmt.Errorf("%w: %q is not in %q", ErrNoMatch, oldString, path)
	default:
		return "", fmt.Errorf("%w: %q is in %q more than once", ErrManyMatches, oldString, path)
	}

	edited := strings.Replace(string(content), oldString, newString, 1)

	if err = p.root.WriteFile(path, []byte(edited), 0o600); err != nil {
		return "", fmt.Errorf("editing %q: %w", path, err)
	}

	return "edited " + path, nil
}

func (p *filePack) listDir(_ context.Context, args map[string]any) (string, error) {
	arguments := tools.NewArguments(args)

	path, err := arguments.GetOptionalString(pathArg, ".")
	if err != nil {
		return "", err
	}

	entries, err := fs.ReadDir(p.root.FS(), path)
	if err != nil {
		return "", fmt.Errorf("listing %q: %w", path, err)
	}

	lines := make([]string, 0, len(entries))

	for _, entry := range entries {
		full := p.fullPath(filepath.Join(path, entry.Name()))

		if entry.IsDir() {
			lines = append(lines, renderDirEntry(entry.Name(), full))

			continue
		}

		info, err := entry.Info()
		if err != nil {
			return "", fmt.Errorf("listing %q: %w", path, err)
		}

		lines = append(lines, renderFileEntry(entry.Name(), info.Size(), full))
	}

	return renderDir(path, lines), nil
}

func renderFileEntry(name string, size int64, path string) string {
	return fmt.Sprintf(`<file name=%q size="%d" path=%q/>`, name, size, path)
}

func renderDirEntry(name string, path string) string {
	return fmt.Sprintf(`<dir name=%q path=%q/>`, name, path)
}

func renderDir(path string, lines []string) string {
	listing := strings.Join(lines, "\n")
	if listing != "" {
		listing += "\n"
	}

	return fmt.Sprintf(`<dir path=%q>`, path) + "\n" + listing + "</dir>"
}

func (p *filePack) searchFiles(ctx context.Context, args map[string]any) (string, error) {
	arguments := tools.NewArguments(args)

	expression, err := arguments.GetString(patternArg)
	if err != nil {
		return "", err
	}

	pattern, err := regexp.Compile(expression)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %w", ErrInvalidPattern, patternArg, err)
	}

	path, err := arguments.GetOptionalString(pathArg, ".")
	if err != nil {
		return "", err
	}

	glob, err := arguments.GetOptionalString(globArg, "")
	if err != nil {
		return "", err
	}

	if _, err = filepath.Match(glob, ""); err != nil {
		return "", fmt.Errorf("%w: %s: %w", ErrInvalidPattern, globArg, err)
	}

	recursive, err := arguments.GetOptionalBool(recursiveArg, true)
	if err != nil {
		return "", err
	}

	limit, err := positiveIntArg(arguments, limitArg, maxSearchMatches)
	if err != nil {
		return "", err
	}

	if _, err = p.root.Stat(path); err != nil {
		return "", fmt.Errorf("searching %q: %w", path, err)
	}

	result, err := search.Files(ctx, search.Config{
		Dir:          p.fullPath(path),
		Pattern:      pattern,
		Glob:         glob,
		Recursive:    recursive,
		MaxMatches:   limit,
		MaxTextBytes: maxMatchTextBytes,
	})
	if errors.Is(err, filepath.ErrBadPattern) {
		return "", fmt.Errorf("%w: %s: %w", ErrInvalidPattern, globArg, err)
	}

	if err != nil {
		return "", fmt.Errorf("searching %q: %w", path, err)
	}

	return p.renderSearch(result), nil
}

func (p *filePack) renderSearch(result *search.Result) string {
	var listing strings.Builder

	files := 0
	previous := ""
	entry := ""

	for _, match := range result.Matches {
		if match.Path != previous {
			previous = match.Path
			entry = p.relativePath(match.Path)
			files++
		}

		listing.WriteString(renderMatch(entry, match))
		listing.WriteString("\n")
	}

	open := fmt.Sprintf(`<search matches="%d" files="%d"%s>`, len(result.Matches), files, truncatedAttr(result.Truncated))

	return open + "\n" + listing.String() + "</search>"
}

func (p *filePack) relativePath(path string) string {
	entry, err := filepath.Rel(p.path, path)
	if err != nil {
		return path
	}

	return entry
}

func renderMatch(path string, match search.Match) string {
	return fmt.Sprintf(`<match path=%q line="%d"%s>%s</match>`, path, match.Line, truncatedAttr(match.Truncated), match.Text)
}

func truncatedAttr(truncated bool) string {
	if !truncated {
		return ""
	}

	return ` truncated="true"`
}

func (p *filePack) deleteFile(_ context.Context, args map[string]any) (string, error) {
	path, err := tools.NewArguments(args).GetString(pathArg)
	if err != nil {
		return "", err
	}

	if err = p.root.Remove(path); err != nil {
		return "", fmt.Errorf("deleting %q: %w", path, err)
	}

	return "deleted " + path, nil
}

func positiveIntArg(arguments *tools.Arguments, name string, fallback int) (int, error) {
	value, err := arguments.GetOptionalInt(name, fallback)
	if err != nil {
		return 0, err
	}

	if value < 1 {
		return 0, fmt.Errorf("%w: %s is %d, expected 1 or more", ErrInvalidRange, name, value)
	}

	return value, nil
}

func readPage(file *os.File, offset int, limit int) ([]string, int, error) {
	var (
		lines     []string
		total     int
		collected int
	)

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), maxFileReadBytes)

	for scanner.Scan() {
		total++

		if total < offset || len(lines) == limit || collected >= maxFileReadBytes {
			continue
		}

		line := scanner.Text()
		collected += len(line) + 1

		lines = append(lines, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, 0, err
	}

	return lines, total, nil
}

func renderPage(lines []string, offset int, total int) string {
	from, to := 0, 0
	if len(lines) > 0 {
		from, to = offset, offset+len(lines)-1
	}

	open := fmt.Sprintf(`<file lines="%d-%d of %d">`, from, to, total)

	return open + "\n" + strings.Join(lines, "\n") + "\n</file>"
}

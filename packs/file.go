package packs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/tools"
)

const (
	readToolName     = "file_read"
	writeToolName    = "file_write"
	editToolName     = "file_edit"
	listToolName     = "file_list"
	deleteToolName   = "file_delete"
	workdirToolName  = "file_workdir"
	pathArg          = "path"
	contentArg       = "content"
	oldStringArg     = "old_string"
	newStringArg     = "new_string"
	offsetArg        = "offset"
	limitArg         = "limit"
	defaultReadLines = 2000
	maxFileReadBytes = 1024 * 1024
)

var fileToolNames = []string{readToolName, writeToolName, editToolName, listToolName, deleteToolName, workdirToolName}

type filePack struct {
	toolBox *tools.ToolBox
	root    *os.Root
	path    string
	once    sync.Once
}

func (p *filePack) Close() error {
	var err error

	p.once.Do(func() {
		for _, name := range fileToolNames {
			p.toolBox.Remove(name)
		}

		err = p.root.Close()
	})

	return err
}

// FileTools registers file access confined to root in m, for an agent that
// produces files without being a coding agent: "file_read" reads a text file a
// page at a time, "file_write" writes one whole, "file_edit" replaces one piece
// of text inside one, "file_list" lists a folder, "file_delete" removes a file
// or a folder that is already empty, and "file_workdir" reports root's absolute
// path, which is how the model names a file it made to a tool that works
// outside root. The returned [ToolPack] removes the six tools again and
// releases the root.
//
// It returns an error, and registers nothing, when root cannot be opened.
//
// The confinement is [os.Root]: paths are relative to root, and one that leaves
// it — by climbing out, by being absolute, or through a symbolic link — is
// refused rather than followed. So a pack rooted at a folder cannot touch the
// rest of the filesystem, which is what separates it from [CodingTools] and
// [ShellTools]; inside that folder the tools carry the authority of the program
// that registered them. The boundary is not a secret: "file_workdir",
// "file_write", "file_list" and the error text all name root's absolute path, so
// register the pack under a folder whose path is safe to disclose.
//
// Two refusals are deliberate. "file_edit" writes nothing unless its text
// appears exactly once, so an edit never lands somewhere the model did not mean.
// "file_delete" will not empty a folder, so nothing recursive happens behind a
// single call.
func FileTools(m *tools.ToolBox, root string) (ToolPack, error) {
	path, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("opening root: %w", err)
	}

	opened, err := os.OpenRoot(path)
	if err != nil {
		return nil, fmt.Errorf("opening root: %w", err)
	}

	pack := &filePack{toolBox: m, root: opened, path: path}

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
	_ = m.Add(readTool, pack.readFile)

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
	_ = m.Add(writeTool, pack.writeFile)

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
	_ = m.Add(editTool, pack.editFile)

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
	_ = m.Add(listTool, pack.listDir)

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
	_ = m.Add(deleteTool, pack.deleteFile)

	workdirTool := llm.Tool{
		Name: workdirToolName,
		Description: "Return the absolute path of the folder these tools work in. The file tools take " +
			"paths relative to it, so use this to build the full path of a file for a tool that works " +
			"outside this folder.",
		Schema: tools.NewObjectBuilder().Build(),
	}
	_ = m.Add(workdirTool, pack.workdir)

	return pack, nil
}

func (p *filePack) workdir(_ context.Context, _ map[string]any) (string, error) {
	return p.path, nil
}

func (p *filePack) fullPath(path string) string {
	return filepath.Join(p.path, path)
}

func (p *filePack) readFile(_ context.Context, args map[string]any) (string, error) {
	arguments := tools.NewArguments(args)

	path, err := arguments.GetString(pathArg)
	if err != nil {
		return "", err
	}

	offset, err := lineArg(arguments, offsetArg, 1)
	if err != nil {
		return "", err
	}

	limit, err := lineArg(arguments, limitArg, defaultReadLines)
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
	path, err := tools.NewArguments(args).GetString(pathArg)
	if err != nil {
		if !errors.Is(err, tools.ErrFieldNotFound) {
			return "", err
		}

		path = "."
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
	return "<file name=\"" + name + "\" size=\"" + strconv.FormatInt(size, 10) + "\" path=\"" + path + "\"/>"
}

func renderDirEntry(name string, path string) string {
	return "<dir name=\"" + name + "\" path=\"" + path + "\"/>"
}

func renderDir(path string, lines []string) string {
	listing := strings.Join(lines, "\n")
	if listing != "" {
		listing += "\n"
	}

	return "<dir path=\"" + path + "\">\n" + listing + "</dir>"
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

func lineArg(arguments *tools.Arguments, name string, fallback int) (int, error) {
	value, err := arguments.GetInt(name)
	if err != nil {
		if errors.Is(err, tools.ErrFieldNotFound) {
			return fallback, nil
		}

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

	page := strconv.Itoa(from) + "-" + strconv.Itoa(to) + " of " + strconv.Itoa(total)

	return "<file lines=\"" + page + "\">\n" + strings.Join(lines, "\n") + "\n</file>"
}

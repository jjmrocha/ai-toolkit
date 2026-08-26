package skills

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/jjmrocha/ai-toolkit/helper"
	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/tools"
)

const (
	loadFileToolName    = "skill_load_file"
	executeFileToolName = "skill_execute_file"
	skillNameArg        = "skill_name"
	pathArg             = "path"
	argsArg             = "args"
	maxOutputBytes      = 1024 * 1024
)

// RegisterTools adds the collection's three tools — [loadToolName],
// [loadFileToolName] and [executeFileToolName] — to tb, so the model can load a
// skill's instructions, read the files that skill ships, and run them. All three
// names are reserved: a tool already registered under any of them is replaced.
// Remove them again with [Collection.UnregisterTools].
func (c *Collection) RegisterTools(tb *tools.ToolBox) {
	loadTool := llm.Tool{
		Name: loadToolName,
		Description: "Load a specialized skill when the task at hand matches one of the available skills. " +
			"The output carries the skill's instructions and the list of files that skill ships.",
		Schema: tools.NewObjectBuilder().
			String(skillNameArg, "The name of the skill from the available skills list", true).
			Build(),
	}
	_ = tb.Add(loadTool, c.loadSkill)

	loadFileTool := llm.Tool{
		Name: loadFileToolName,
		Description: "Read one of the files a skill ships, using a path from the file list returned by " +
			loadToolName + ".",
		Schema: tools.NewObjectBuilder().
			String(skillNameArg, "The name of the skill the file belongs to", true).
			String(pathArg, "A path from the skill's file list", true).
			Build(),
	}
	_ = tb.Add(loadFileTool, c.loadSkillFile)

	executeFileTool := llm.Tool{
		Name: executeFileToolName,
		Description: "Execute one of the files a skill ships, using a path from the file list returned by " +
			loadToolName + ". The file runs from the skill's own folder; its combined output and exit status " +
			"are returned. A non-zero exit status is a result, not a failure.",
		Schema: tools.NewObjectBuilder().
			String(skillNameArg, "The name of the skill the file belongs to", true).
			String(pathArg, "A path from the skill's file list", true).
			ArrayOfStrings(argsArg, "Arguments passed to the file", false).
			Build(),
	}
	_ = tb.Add(executeFileTool, c.executeSkillFile)
}

// UnregisterTools removes the collection's three tools from tb, leaving every
// other tool in place. It is a no-op when they are not registered.
func (c *Collection) UnregisterTools(tb *tools.ToolBox) {
	tb.Remove(loadToolName)
	tb.Remove(loadFileToolName)
	tb.Remove(executeFileToolName)
}

func (c *Collection) loadSkill(_ context.Context, args map[string]any) (string, error) {
	name, err := tools.NewArguments(args).GetString(skillNameArg)
	if err != nil {
		return "", err
	}

	s, err := c.lookup(name)
	if err != nil {
		return "", err
	}

	lines := []string{
		`<skill name="` + s.name + `">`,
		strings.TrimSpace(s.body),
	}

	if len(s.files) > 0 {
		lines = append(lines, "", "Files available via "+loadFileToolName+":")

		for _, file := range s.files {
			lines = append(lines, "  "+file)
		}
	}

	lines = append(lines, "</skill>")

	return strings.Join(lines, "\n"), nil
}

func (c *Collection) loadSkillFile(_ context.Context, args map[string]any) (string, error) {
	arguments := tools.NewArguments(args)

	name, err := arguments.GetString(skillNameArg)
	if err != nil {
		return "", err
	}

	path, err := arguments.GetString(pathArg)
	if err != nil {
		return "", err
	}

	s, err := c.lookup(name)
	if err != nil {
		return "", err
	}

	if err = s.checkFile(path); err != nil {
		return "", err
	}

	root, err := os.OpenRoot(s.skillPath)
	if err != nil {
		return "", fmt.Errorf("opening folder of skill %q: %w", name, hidePath(err))
	}

	defer func() { _ = root.Close() }()

	content, err := fs.ReadFile(root.FS(), path)
	if err != nil {
		return "", fmt.Errorf("reading %q from skill %q: %w", path, name, err)
	}

	return string(content), nil
}

func (c *Collection) executeSkillFile(ctx context.Context, args map[string]any) (string, error) {
	arguments := tools.NewArguments(args)

	name, err := arguments.GetString(skillNameArg)
	if err != nil {
		return "", err
	}

	scriptPath, err := arguments.GetString(pathArg)
	if err != nil {
		return "", err
	}

	fileArgs, err := arguments.GetArrayOfStrings(argsArg)
	if err != nil && !errors.Is(err, tools.ErrFieldNotFound) {
		return "", err
	}

	s, err := c.lookup(name)
	if err != nil {
		return "", err
	}

	if err = s.checkFile(scriptPath); err != nil {
		return "", err
	}

	result, err := helper.Run(ctx, helper.RunConfig{
		Path:           filepath.Join(s.skillPath, scriptPath),
		Args:           fileArgs,
		Dir:            s.skillPath,
		MaxOutputBytes: maxOutputBytes,
	})
	if err != nil {
		return "", fmt.Errorf("executing %q from skill %q: %w", scriptPath, name, hidePath(err))
	}

	return renderExecution(result), nil
}

func hidePath(err error) error {
	if pathErr, ok := errors.AsType[*fs.PathError](err); ok {
		return pathErr.Err
	}

	return err
}

func (s skill) checkFile(path string) error {
	if slices.Contains(s.files, path) {
		return nil
	}

	return fmt.Errorf("%w: %q in skill %q; files in this skill: %s",
		ErrFileNotFound, path, s.name, strings.Join(s.files, ", "))
}

func renderExecution(result helper.RunResult) string {
	open := "<output>"
	if result.Truncated {
		open = `<output truncated="true">`
	}

	lines := []string{
		"exit status: " + strconv.Itoa(result.ExitCode),
		open,
		strings.Join(result.Output, "\n"),
		"</output>",
	}

	return strings.Join(lines, "\n")
}

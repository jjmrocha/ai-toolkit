package skills

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"

	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/tools"
)

// LoadFileToolName is the name of the tool that reads one file from a skill's
// folder. It is reserved on the same terms as [LoadToolName].
const LoadFileToolName = "skill_load_file"

const (
	skillNameArg = "skill_name"
	pathArg      = "path"
)

// RegisterTools adds the collection's two tools — [LoadToolName] and
// [LoadFileToolName] — to tb, so the model can load a skill's instructions and
// read the files that skill ships. Both names are reserved: a tool already
// registered under either is replaced. Remove them again with
// [Collection.UnregisterTools].
func (c *Collection) RegisterTools(tb *tools.ToolBox) {
	loadTool := llm.Tool{
		Name: LoadToolName,
		Description: "Load a specialized skill when the task at hand matches one of the available skills. " +
			"The output carries the skill's instructions and the list of files that skill ships.",
		Schema: tools.NewObjectBuilder().
			String(skillNameArg, "The name of the skill from the available skills list", true).
			Build(),
	}
	_ = tb.Add(loadTool, c.loadSkill)

	loadFileTool := llm.Tool{
		Name: LoadFileToolName,
		Description: "Read one of the files a skill ships, using a path from the file list returned by " +
			LoadToolName + ".",
		Schema: tools.NewObjectBuilder().
			String(skillNameArg, "The name of the skill the file belongs to", true).
			String(pathArg, "A path from the skill's file list", true).
			Build(),
	}
	_ = tb.Add(loadFileTool, c.loadSkillFile)
}

// UnregisterTools removes the collection's two tools from tb, leaving every
// other tool in place. It is a no-op when they are not registered.
func (c *Collection) UnregisterTools(tb *tools.ToolBox) {
	tb.Remove(LoadToolName)
	tb.Remove(LoadFileToolName)
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
		"# Skill: " + s.name,
		"",
		strings.TrimSpace(s.body),
	}

	if len(s.files) > 0 {
		lines = append(lines, "", "Files available via "+LoadFileToolName+":")

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

	if !slices.Contains(s.files, path) {
		return "", fmt.Errorf("%w: %q in skill %q; files in this skill: %s",
			ErrFileNotFound, path, name, strings.Join(s.files, ", "))
	}

	root, err := os.OpenRoot(s.path)
	if err != nil {
		return "", fmt.Errorf("opening folder of skill %q: %w", name, err)
	}

	defer func() { _ = root.Close() }()

	content, err := fs.ReadFile(root.FS(), path)
	if err != nil {
		return "", fmt.Errorf("reading %q from skill %q: %w", path, name, err)
	}

	return string(content), nil
}

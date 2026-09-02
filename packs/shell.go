package packs

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jjmrocha/ai-toolkit/internal/helper"
	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/tools"
)

const (
	shellToolName       = "shell_run"
	commandArg          = "command"
	workdirArg          = "workdir"
	timeoutArg          = "timeout_ms"
	shellPath           = "/bin/sh"
	defaultShellTimeout = 2 * time.Minute
	maxShellTimeout     = 10 * time.Minute
	maxShellOutputBytes = 1024 * 1024
)

type shellPack struct {
	toolBox *tools.ToolBox
}

func (p *shellPack) Close() error {
	p.toolBox.Remove(shellToolName)

	return nil
}

// ShellTools registers a single tool, "shell_run", that runs a command line
// with /bin/sh in m. It needs nothing beyond the shell, and the returned
// [ToolPack] removes the tool again. Nothing is launched, so its Close only
// unregisters.
//
// The command runs with the authority of the program that registered the tool:
// its whole filesystem, its environment and the credentials in it. Register it
// only for a model and a conversation you would trust with a shell.
//
// The call chooses the working directory and a timeout of up to ten minutes,
// two by default. A command that outlasts its timeout is stopped, and the model
// is told so rather than handed an error. Output is capped at 1 MiB, after
// which the command is stopped and the result is marked truncated.
func ShellTools(m *tools.ToolBox) ToolPack {
	tool := llm.Tool{
		Name: shellToolName,
		Description: "Run a command line with /bin/sh and return its combined output and exit status. " +
			"Use it for terminal work such as git, build tools and package managers; prefer the dedicated " +
			"tools for reading, writing and searching files. A non-zero exit status is a result, not a " +
			"failure. The command gets no stdin, so one that reads input sees end of input at once.",
		Schema: tools.NewObjectBuilder().
			String(commandArg, "The command line to run", true).
			String(workdirArg, "Directory to run the command in, defaulting to the current one", false).
			Integer(timeoutArg, "Milliseconds to let the command run, from 1 to "+
				strconv.FormatInt(maxShellTimeout.Milliseconds(), 10)+", defaulting to "+
				strconv.FormatInt(defaultShellTimeout.Milliseconds(), 10), false).
			Build(),
	}
	_ = m.Add(tool, runShellCommand)

	return &shellPack{toolBox: m}
}

func runShellCommand(ctx context.Context, args map[string]any) (string, error) {
	arguments := tools.NewArguments(args)

	command, err := arguments.GetString(commandArg)
	if err != nil {
		return "", err
	}

	dir, err := arguments.GetString(workdirArg)
	if err != nil && !errors.Is(err, tools.ErrFieldNotFound) {
		return "", err
	}

	timeout, err := shellTimeout(arguments)
	if err != nil {
		return "", err
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := helper.Run(runCtx, helper.RunConfig{
		Path:           shellPath,
		Args:           []string{"-c", command},
		Dir:            dir,
		MaxOutputBytes: maxShellOutputBytes,
	})
	if err != nil {
		if ctx.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
			return renderShellTimeout(timeout), nil
		}

		return "", fmt.Errorf("executing %q: %w", command, err)
	}

	return renderShellResult(result), nil
}

func shellTimeout(arguments *tools.Arguments) (time.Duration, error) {
	millis, err := arguments.GetInt(timeoutArg)
	if err != nil {
		if errors.Is(err, tools.ErrFieldNotFound) {
			return defaultShellTimeout, nil
		}

		return 0, err
	}

	timeout := time.Duration(millis) * time.Millisecond
	if timeout <= 0 || timeout > maxShellTimeout {
		return 0, fmt.Errorf("%w: %d ms, expected 1 to %d", ErrInvalidTimeout, millis,
			maxShellTimeout.Milliseconds())
	}

	return timeout, nil
}

func renderShellTimeout(timeout time.Duration) string {
	return "timed out after " + strconv.FormatInt(timeout.Milliseconds(), 10) +
		" ms, retry with a larger " + timeoutArg + " if the command needs longer"
}

func renderShellResult(result helper.RunResult) string {
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

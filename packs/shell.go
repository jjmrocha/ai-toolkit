package packs

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jjmrocha/ai-toolkit/internal/command"
	"github.com/jjmrocha/ai-toolkit/llm"
	"github.com/jjmrocha/ai-toolkit/mcp"
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

const shellInstruction = `Reach for this pack for terminal work — building,
testing, linting, git and package managers. Read and change files with the tools
you have for that rather than by printing or rewriting them with a command, and
take each command from the user's instructions or the repository's own Makefile,
README or CI configuration rather than inventing one.`

type shellPack struct {
	toolBox *tools.ToolBox
	once    sync.Once
}

func (p *shellPack) Close() error {
	p.once.Do(func() {
		p.toolBox.Remove(shellToolName)
	})

	return nil
}

func (p *shellPack) Instructions(_ context.Context) (*mcp.Instruction, error) {
	return &mcp.Instruction{
		Name: "shell",
		Text: shellInstruction,
	}, nil
}

// ShellTools registers "shell_run" in m, which runs a command line with
// /bin/sh. Nothing is launched, so [ToolPack.Close] only unregisters. If m
// rejects the registration, ShellTools returns the error from
// [tools.ToolBox.Add].
//
// Commands run with the program's authority: its whole filesystem, its
// environment and any credentials in it. Register it only for a model you would
// trust with a shell.
//
// The call picks the working directory and a timeout of up to ten minutes, two
// by default. A command that runs past its timeout is stopped and the model is
// told so; that is not an error. Output is capped at 1 MiB, after which the
// command is stopped and the result marked truncated.
func ShellTools(m *tools.ToolBox) (ToolPack, error) {
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
	err := m.Add(tool, runShellCommand)
	if err != nil {
		return nil, err
	}

	return &shellPack{toolBox: m}, nil
}

func runShellCommand(ctx context.Context, args map[string]any) (string, error) {
	arguments := tools.NewArguments(args)

	commandLine, err := arguments.GetString(commandArg)
	if err != nil {
		return "", err
	}

	dir, err := arguments.GetOptionalString(workdirArg, "")
	if err != nil {
		return "", err
	}

	timeout, err := shellTimeout(arguments)
	if err != nil {
		return "", err
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := command.Run(runCtx, command.RunConfig{
		Path:           shellPath,
		Args:           []string{"-c", commandLine},
		Dir:            dir,
		MaxOutputBytes: maxShellOutputBytes,
	})
	if err != nil {
		if ctx.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
			return renderShellTimeout(timeout), nil
		}

		return "", fmt.Errorf("executing %q: %w", commandLine, err)
	}

	return renderShellResult(result), nil
}

func shellTimeout(arguments *tools.Arguments) (time.Duration, error) {
	millis, err := arguments.GetOptionalInt(timeoutArg, int(defaultShellTimeout.Milliseconds()))
	if err != nil {
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

func renderShellResult(result *command.RunResult) string {
	open := fmt.Sprintf(`<output%s>`, truncatedAttr(result.Truncated))

	lines := []string{
		"exit status: " + strconv.Itoa(result.ExitCode),
		open,
		strings.Join(result.Output, "\n"),
		"</output>",
	}

	return strings.Join(lines, "\n")
}

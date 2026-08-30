package helper

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// RunConfig describes the command [Run] launches. Path and Args are run with
// os/exec without a shell, so they are trusted input: supply them from operator
// configuration, never from an untrusted source.
type RunConfig struct {
	// Path is the executable to launch.
	Path string
	// Args are the arguments passed to Path.
	Args []string
	// Dir is the working directory the process runs in. An empty Dir runs it in
	// the calling process's working directory.
	Dir string
	// MaxOutputBytes is how much output [Run] collects before it stops the
	// command, counting each line and the newline that followed it. The line
	// that passes the limit is kept, so [RunResult.Output] may run over it by
	// that much, and [RunResult] is marked truncated. A zero MaxOutputBytes
	// collects everything.
	MaxOutputBytes int
}

// RunResult is what [Run] collected from a command that ran to completion.
type RunResult struct {
	// ExitCode is the status the command exited with.
	ExitCode int
	// Output holds the command's output, one line at a time with the newline
	// removed.
	Output []string
	// Truncated reports whether the command was stopped for writing more than
	// [RunConfig.MaxOutputBytes]. [RunResult.Output] then holds only the start
	// of its output, and [RunResult.ExitCode] describes the kill, not the
	// command's own choice.
	Truncated bool
}

// Run launches the command cfg describes, collects its output until it ends,
// and waits for it to exit. The command's stderr is always merged into the
// output, in the order the command wrote it; [NewProcess] is the way to read
// stdout on its own.
//
// A non-zero exit status is part of the [RunResult], not an error. Run returns
// an error when the command cannot be started, when ctx ends first, or when
// waiting on it fails for any other reason. Unless [RunConfig.MaxOutputBytes]
// says otherwise, Run keeps everything the command writes.
func Run(ctx context.Context, cfg RunConfig) (RunResult, error) {
	exited := make(chan error, 1)

	proc := ProcessConfig{
		Path:          cfg.Path,
		Args:          cfg.Args,
		Dir:           cfg.Dir,
		OnExit:        func(err error) { exited <- err },
		IncludeStderr: true,
	}

	process, err := NewProcess(proc)
	if err != nil {
		return RunResult{}, err
	}

	defer process.Close()

	output, truncated, err := collect(ctx, process, cfg.MaxOutputBytes)
	if err != nil {
		return RunResult{}, err
	}

	select {
	case err := <-exited:
		exitCode, err := exitStatus(err, cfg.Path)
		if err != nil {
			return RunResult{}, err
		}

		return RunResult{ExitCode: exitCode, Output: output, Truncated: truncated}, nil
	case <-ctx.Done():
		return RunResult{}, ctx.Err()
	}
}

func collect(ctx context.Context, process *Process, maxBytes int) ([]string, bool, error) {
	var (
		output    []string
		collected int
	)

	for {
		select {
		case line, open := <-process.Output():
			if !open {
				return output, false, nil
			}

			collected += len(line) + 1
			output = append(output, line)

			if maxBytes > 0 && collected > maxBytes {
				process.Close()

				return output, true, nil
			}
		case <-ctx.Done():
			return nil, false, ctx.Err()
		}
	}
}

func exitStatus(err error, command string) (int, error) {
	if err == nil {
		return 0, nil
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return 0, fmt.Errorf("waiting for %q: %w", command, err)
	}

	return exitErr.ExitCode(), nil
}

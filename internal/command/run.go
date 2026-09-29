package command

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// RunConfig describes the command [Run] launches. Path and Args run through
// os/exec with no shell, so they are trusted input: take them from operator
// configuration, never from an untrusted source.
type RunConfig struct {
	// Path is the executable to launch.
	Path string
	// Args are the arguments passed to Path.
	Args []string
	// Dir is the working directory. Empty means the caller's.
	Dir string
	// Env is the environment, as KEY=VALUE entries. Nil passes the caller's whole
	// environment, credentials included; [InheritedEnv] builds a filtered one.
	// Empty but not nil means no environment at all.
	Env []string
	// MaxOutputBytes is how much output [Run] collects, counting each line and its
	// newline, before it stops the command and marks the [RunResult] truncated. The
	// line that crosses the limit is kept, so the output can exceed it by one line.
	// Zero collects everything.
	MaxOutputBytes int
}

// RunResult is what [Run] collected from a command that finished.
type RunResult struct {
	// ExitCode is the status the command exited with.
	ExitCode int
	// Output is the command's output, one line per element, without newlines.
	Output []string
	// Truncated reports whether the command was stopped for exceeding
	// [RunConfig.MaxOutputBytes]. Output then holds only the start, and ExitCode
	// reflects the kill, not the command.
	Truncated bool
}

// Run launches the command, collects its output until it ends, and waits for
// it to exit. Stderr is always merged into the output, in the order it was
// written; use [NewProcess] to read stdout alone.
//
// A non-zero exit status goes in the [RunResult], not the error. Run returns an
// error when the command cannot start, when ctx ends first, or when waiting on
// it fails for another reason.
func Run(ctx context.Context, cfg RunConfig) (*RunResult, error) {
	exited := make(chan error, 1)

	proc := ProcessConfig{
		Path:          cfg.Path,
		Args:          cfg.Args,
		Dir:           cfg.Dir,
		Env:           cfg.Env,
		OnExit:        func(err error) { exited <- err },
		IncludeStderr: true,
	}

	process, err := NewProcess(proc)
	if err != nil {
		return nil, err
	}

	defer process.Close()

	output, truncated, err := collect(ctx, process, cfg.MaxOutputBytes)
	if err != nil {
		return nil, err
	}

	select {
	case err := <-exited:
		exitCode, err := exitStatus(err, cfg.Path)
		if err != nil {
			return nil, err
		}

		result := RunResult{
			ExitCode:  exitCode,
			Output:    output,
			Truncated: truncated,
		}

		return &result, nil
	case <-ctx.Done():
		return nil, ctx.Err()
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

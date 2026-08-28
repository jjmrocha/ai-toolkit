package helper

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultStopTimeout = 3 * time.Second
	initialLineBytes   = 4 * 1024
)

var (
	// ErrProcessClosed is returned by [Process.Write] when the process has
	// exited or is being shut down, so the message can no longer be delivered.
	ErrProcessClosed = errors.New("process closed")
	// ErrInvalidMessage is returned by [Process.Write] when the message holds a
	// newline, which would be read as the end of the line.
	ErrInvalidMessage = errors.New("invalid message")
	// ErrInputNotAllowed is returned by [Process.Write] when the process was
	// built without [ProcessConfig.AllowInput], so it has no stdin to write to.
	ErrInputNotAllowed = errors.New("input not allowed")
)

// ExitNotification is called once with the result of waiting on the process:
// nil when it exited cleanly, otherwise the error from the wait. An exit status
// is recovered from that error with [errors.As] on an [os/exec.ExitError].
type ExitNotification func(error)

// ProcessConfig describes the process [NewProcess] launches. Path and Args are
// run with os/exec without a shell, so they are trusted input: supply them from
// operator configuration, never from an untrusted source.
type ProcessConfig struct {
	// Path is the executable to launch.
	Path string
	// Args are the arguments passed to Path.
	Args []string
	// Dir is the working directory the process runs in. An empty Dir runs it in
	// the calling process's working directory.
	Dir string
	// IncludeStderr merges the process's stderr into [Process.Output], in the
	// order the process wrote it. When false, Output carries stdout alone and
	// stderr is discarded.
	IncludeStderr bool
	// AllowInput gives the process a stdin for [Process.Write] to send to.
	// Without it the process reads from the null device, so anything waiting on
	// stdin sees end of input at once.
	AllowInput bool
	// OnExit is notified when the process exits. A nil OnExit is not called.
	OnExit ExitNotification
}

// Process is a running child process whose stdout, and optionally stderr, is
// delivered line by line on [Process.Output]. Build one with [NewProcess] and
// release it with [Process.Close].
type Process struct {
	cmd        *exec.Cmd
	allowInput bool
	outgoing   chan string
	incoming   chan string
	closing    chan struct{}
	exited     chan struct{}
	closeOnce  sync.Once
}

// NewProcess launches the process cfg describes and starts reading its output.
// The returned [Process] owns the child until [Process.Close] is called, which
// the caller must do even when the process exits on its own.
func NewProcess(cfg ProcessConfig) (*Process, error) {
	cmd := exec.Command(cfg.Path, cfg.Args...) //nolint:gosec // command and args are operator-provided configuration
	cmd.Dir = cfg.Dir

	var stdin io.WriteCloser

	if cfg.AllowInput {
		pipe, err := cmd.StdinPipe()
		if err != nil {
			return nil, fmt.Errorf("opening process stdin: %w", err)
		}

		stdin = pipe
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("opening process output: %w", err)
	}

	defer func() {
		_ = writer.Close()
	}()

	cmd.Stdout = writer

	if cfg.IncludeStderr {
		cmd.Stderr = writer
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("fail to start process: %w", err)
	}

	p := &Process{
		cmd:        cmd,
		allowInput: cfg.AllowInput,
		outgoing:   make(chan string),
		incoming:   make(chan string),
		closing:    make(chan struct{}),
		exited:     make(chan struct{}),
	}

	go p.reap(cfg.OnExit)
	go p.readLoop(reader)

	if stdin != nil {
		go p.writeLoop(stdin)
	}

	return p, nil
}

func (p *Process) reap(onExit ExitNotification) {
	defer close(p.exited)

	err := p.cmd.Wait()

	if onExit != nil {
		onExit(err)
	}
}

func (p *Process) writeLoop(stdin io.WriteCloser) {
	defer func() {
		_ = stdin.Close()
	}()

	for {
		select {
		case <-p.closing:
			return
		case msg := <-p.outgoing:
			if _, err := fmt.Fprintln(stdin, msg); err != nil {
				return
			}
		}
	}
}

func (p *Process) readLoop(stdout io.ReadCloser) {
	defer func() {
		_ = stdout.Close()
		close(p.incoming)
	}()

	scanner := bufio.NewScanner(stdout)
	buffer := make([]byte, 0, initialLineBytes)
	scanner.Buffer(buffer, math.MaxInt)

	for scanner.Scan() {
		line := scanner.Text()

		select {
		case p.incoming <- line:
		case <-p.closing:
			return
		}
	}

	go p.Close()
}

// Close stops the process and releases everything [NewProcess] started. A
// process that has already exited is reaped at once. One that is still running
// is given a grace period to leave on its own, then sent SIGTERM; if it
// outlasts a second grace period it is sent SIGKILL. Close returns once the
// process has been reaped, so it blocks for as long as those periods take.
// Calling Close more than once is safe.
func (p *Process) Close() {
	p.closeOnce.Do(func() {
		close(p.closing)
		p.stopProcess()
		<-p.exited
	})
}

func (p *Process) stopProcess() {
	for _, sig := range []os.Signal{syscall.SIGTERM, syscall.SIGKILL} {
		select {
		case <-p.exited:
			return
		case <-time.After(defaultStopTimeout):
		}

		_ = p.cmd.Process.Signal(sig)
	}
}

// Running reports whether the process is still running.
func (p *Process) Running() bool {
	select {
	case <-p.exited:
		return false
	default:
		return true
	}
}

// Write sends msg to the process's stdin, followed by a newline. It returns
// [ErrInvalidMessage] when msg holds a newline of its own,
// [ErrInputNotAllowed] when the process was built without
// [ProcessConfig.AllowInput], [ErrProcessClosed] once the process has exited or
// Close has been called, and ctx's error when ctx ends before the message is
// handed over.
func (p *Process) Write(ctx context.Context, msg string) error {
	if !p.allowInput {
		return fmt.Errorf("%w: the process was built without a stdin", ErrInputNotAllowed)
	}

	if strings.ContainsRune(msg, '\n') {
		return fmt.Errorf("%w: message contains a newline", ErrInvalidMessage)
	}

	select {
	case p.outgoing <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-p.closing:
		return fmt.Errorf("%w: process closing", ErrProcessClosed)
	case <-p.exited:
		return fmt.Errorf("%w: process exited", ErrProcessClosed)
	}
}

// Output returns the channel carrying the process's output, one line at a time
// with the newline removed. It is closed when the process's output ends.
func (p *Process) Output() <-chan string {
	return p.incoming
}

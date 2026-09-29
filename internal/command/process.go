package command

import (
	"bufio"
	"context"
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

// ExitNotification is called once with the result of waiting on the process:
// nil for a clean exit, otherwise the wait error. Get the exit status from it
// with [errors.As] on an [os/exec.ExitError].
type ExitNotification func(error)

// ProcessConfig describes the process [NewProcess] launches. Path and Args run
// through os/exec with no shell, so they are trusted input: take them from
// operator configuration, never from an untrusted source.
type ProcessConfig struct {
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
	// IncludeStderr merges stderr into [Process.Output], in the order the process
	// wrote it. When false, Output is stdout only and stderr is discarded.
	IncludeStderr bool
	// AllowInput gives the process a stdin for [Process.Write]. Without it, stdin
	// is the null device, so a read sees end of input at once.
	AllowInput bool
	// OnExit is called when the process exits, if not nil.
	OnExit ExitNotification
}

// Process is a running child process whose stdout, and optionally stderr,
// arrives line by line on [Process.Output]. Create one with [NewProcess] and
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

// NewProcess launches the process and starts reading its output. The caller
// must call [Process.Close], even if the process exits on its own.
func NewProcess(cfg ProcessConfig) (*Process, error) {
	cmd := exec.Command(cfg.Path, cfg.Args...) //nolint:gosec // command and args are operator-provided configuration
	cmd.Dir = cfg.Dir
	cmd.Env = cfg.Env

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

// Close stops the process and releases everything [NewProcess] started. An
// exited process is reaped at once. A running one gets a grace period to exit,
// then SIGTERM, then after a second grace period SIGKILL. Close blocks until
// the process is reaped. It is safe to call more than once.
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

// Write sends msg and a newline to the process's stdin. It returns
// [ErrInvalidMessage] when msg has a newline, [ErrInputNotAllowed] when
// [ProcessConfig.AllowInput] was not set, [ErrProcessClosed] once the process
// has exited or Close was called, and ctx's error when ctx ends before the
// message is sent.
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

// Output returns the channel of output lines, without newlines. It is closed
// when the output ends.
func (p *Process) Output() <-chan string {
	return p.incoming
}

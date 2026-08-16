package mcp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	stopTimeout         = 2 * time.Second
	initialMessageBytes = 4 * 1024
	maxMessageBytes     = 1024 * 1024 // 1 MiB
)

type stdioTransport struct {
	cmd       *exec.Cmd
	outgoing  chan string
	incoming  chan string
	closing   chan struct{}
	exited    chan struct{}
	closeOnce sync.Once
}

type disconnectedNotification func(error)

func newStdioTransport(command string, args []string, onExit disconnectedNotification) (*stdioTransport, error) {
	cmd := exec.Command(command, args...) //nolint:gosec // command and args are operator-provided server config

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("opening server stdin: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("opening server stdout: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("fail to start process: %w", err)
	}

	t := &stdioTransport{
		cmd:      cmd,
		outgoing: make(chan string),
		incoming: make(chan string),
		closing:  make(chan struct{}),
		exited:   make(chan struct{}),
	}

	go t.reap(onExit)
	go t.writeLoop(stdin)
	go t.readLoop(stdout)

	return t, nil
}

func (t *stdioTransport) reap(onExit disconnectedNotification) {
	defer close(t.exited)

	err := t.cmd.Wait()

	if onExit != nil {
		onExit(err)
	}
}

func (t *stdioTransport) writeLoop(stdin io.WriteCloser) {
	defer func() {
		_ = stdin.Close()
	}()

	for {
		select {
		case <-t.closing:
			return
		case msg := <-t.outgoing:
			if _, err := fmt.Fprintln(stdin, msg); err != nil {
				return
			}
		}
	}
}

func (t *stdioTransport) readLoop(stdout io.ReadCloser) {
	defer close(t.incoming)

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, initialMessageBytes), maxMessageBytes)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		select {
		case t.incoming <- line:
		case <-t.closing:
			return
		}
	}

	go t.Close()
}

func (t *stdioTransport) Close() {
	t.closeOnce.Do(func() {
		close(t.closing)
		t.stopServer()
		<-t.exited
	})
}

func (t *stdioTransport) stopServer() {
	for _, sig := range []os.Signal{syscall.SIGTERM, syscall.SIGKILL} {
		select {
		case <-t.exited:
			return
		case <-time.After(stopTimeout):
		}

		_ = t.cmd.Process.Signal(sig)
	}
}

func (t *stdioTransport) Running() bool {
	select {
	case <-t.exited:
		return false
	default:
		return true
	}
}

func (t *stdioTransport) Write(ctx context.Context, msg string) error {
	if strings.ContainsRune(msg, '\n') {
		return fmt.Errorf("%w: message contains a newline", ErrInvalidMessage)
	}

	select {
	case t.outgoing <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-t.closing:
		return fmt.Errorf("%w: connection closing", ErrProcessClosed)
	case <-t.exited:
		return fmt.Errorf("%w: process exited", ErrProcessClosed)
	}
}

func (t *stdioTransport) Reader() <-chan string {
	return t.incoming
}

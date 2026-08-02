package mcp

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	stopTimeout     = 2 * time.Second
	maxMessageBytes = 1024 * 1024 // 1 MiB
)

type cmdIO struct {
	cmd       *exec.Cmd
	inbox     chan string
	outbox    chan string
	closing   chan struct{}
	exited    chan struct{}
	closeOnce sync.Once
}

type disconnectedNotification func(error)

func newCmdIO(command string, args []string, fn disconnectedNotification) (*cmdIO, error) {
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

	inbox := make(chan string)
	outbox := make(chan string)
	closing := make(chan struct{})
	exited := make(chan struct{})

	// We don't care for messages still on the pipe.
	go func() {
		defer close(exited)
		err := cmd.Wait()

		if fn != nil {
			fn(err)
		}
	}()

	go func() {
		defer func() {
			_ = stdin.Close()
		}()

		for {
			select {
			case <-closing:
				return
			case msg := <-inbox:
				if _, err := fmt.Fprintln(stdin, msg); err != nil {
					return
				}
			}
		}
	}()

	go func() {
		defer close(outbox)

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), maxMessageBytes)

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			select {
			case outbox <- line:
			case <-closing:
				return
			}
		}
	}()

	return &cmdIO{
		cmd:     cmd,
		inbox:   inbox,
		outbox:  outbox,
		closing: closing,
		exited:  exited,
	}, nil
}

func (c *cmdIO) Close() {
	c.closeOnce.Do(func() {
		close(c.closing)
		c.stopServer()
		<-c.exited
	})
}

func (c *cmdIO) stopServer() {
	for _, sig := range []os.Signal{syscall.SIGTERM, syscall.SIGKILL} {
		select {
		case <-c.exited:
			return
		case <-time.After(stopTimeout):
		}

		_ = c.cmd.Process.Signal(sig)
	}
}

func (c *cmdIO) Running() bool {
	select {
	case <-c.exited:
		return false
	default:
		return true
	}
}

func (c *cmdIO) Write(ctx context.Context, msg string) error {
	if strings.ContainsRune(msg, '\n') {
		return fmt.Errorf("%w: message contains a newline", ErrInvalidMessage)
	}

	select {
	case c.inbox <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.closing:
		return fmt.Errorf("%w: connection closing", ErrProcessClosed)
	case <-c.exited:
		return fmt.Errorf("%w: process exited", ErrProcessClosed)
	}
}

func (c *cmdIO) Reader() <-chan string {
	return c.outbox
}

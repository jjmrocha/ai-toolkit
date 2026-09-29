package command

import "errors"

var (
	// ErrProcessClosed is returned by [Process.Write] when the process has exited
	// or is shutting down.
	ErrProcessClosed = errors.New("process closed")
	// ErrInvalidMessage is returned by [Process.Write] when the message has a
	// newline, which would end the line early.
	ErrInvalidMessage = errors.New("invalid message")
	// ErrInputNotAllowed is returned by [Process.Write] when the process has no
	// stdin because [ProcessConfig.AllowInput] was not set.
	ErrInputNotAllowed = errors.New("input not allowed")
)

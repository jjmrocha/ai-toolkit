package command

import "errors"

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

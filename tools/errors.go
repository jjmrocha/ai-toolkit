package tools

import "errors"

var (
	// ErrToolNotFound is returned by ToolBox.ExecuteTool when no registered
	// tool matches the requested call name.
	ErrToolNotFound = errors.New("tool not found")
)

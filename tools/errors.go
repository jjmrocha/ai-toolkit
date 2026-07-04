package tools

import "errors"

var (
	// ErrToolNotFound is returned by ToolBox.ExecuteTool when no registered
	// tool matches the requested call name.
	ErrToolNotFound = errors.New("tool not found")

	// ErrInvalidToolName is returned by ToolBox.AddTool when the tool name is
	// empty or contains characters outside the set accepted by the providers.
	ErrInvalidToolName = errors.New("invalid tool name")
)

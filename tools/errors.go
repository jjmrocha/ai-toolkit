package tools

import "errors"

// Errors returned by the tools package.
var (
	// ErrToolNotFound is returned by [ToolBox.Execute] when no registered
	// tool matches the requested call name.
	ErrToolNotFound = errors.New("tool not found")

	// ErrInvalidToolName is returned by [ToolBox.Add] when the tool name is
	// empty or contains characters outside the set accepted by the providers.
	ErrInvalidToolName = errors.New("invalid tool name")

	// ErrNilHandler is returned by [ToolBox.Add] when the handler is nil.
	ErrNilHandler = errors.New("tool handler is nil")

	// ErrFieldNotFound is returned by the [Arguments] accessors when the
	// requested field is absent from the tool call's arguments.
	ErrFieldNotFound = errors.New("field not found")

	// ErrInvalidFieldType is returned by the [Arguments] accessors when the
	// requested field holds a value of another type, or, for the array
	// accessors, when one of its elements does.
	ErrInvalidFieldType = errors.New("invalid field type")
)

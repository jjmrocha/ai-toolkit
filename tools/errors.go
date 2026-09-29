package tools

import "errors"

// Errors returned by the tools package.
var (
	// ErrToolNotFound is returned by [ToolBox.Execute] when no registered
	// tool matches the requested call name.
	ErrToolNotFound = errors.New("tool not found")

	// ErrInvalidToolName is returned by [ToolBox.Add] when the tool name is
	// empty or has characters the providers reject.
	ErrInvalidToolName = errors.New("invalid tool name")

	// ErrNilHandler is returned by [ToolBox.Add] when the handler is nil.
	ErrNilHandler = errors.New("tool handler is nil")

	// ErrFieldNotFound is returned by the [Arguments] accessors when the field is
	// missing.
	ErrFieldNotFound = errors.New("field not found")

	// ErrInvalidFieldType is returned by the [Arguments] accessors when the field,
	// or for the array accessors one of its elements, has another type.
	ErrInvalidFieldType = errors.New("invalid field type")
)

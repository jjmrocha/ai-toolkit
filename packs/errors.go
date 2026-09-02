package packs

import "errors"

// ErrInvalidTimeout is returned by the shell tool [ShellTools] registers when
// the call asks for a timeout outside the range the tool accepts.
var ErrInvalidTimeout = errors.New("invalid timeout")

// ErrInvalidRange is returned by the file tools [FileTools] registers when the
// call asks for a line offset or a line count below one.
var ErrInvalidRange = errors.New("invalid range")

// ErrNoMatch is returned by the file tool "file_edit" [FileTools] registers when
// the text the call asks to replace is not in the file.
var ErrNoMatch = errors.New("no match")

// ErrManyMatches is returned by the file tool "file_edit" [FileTools] registers
// when the text the call asks to replace is in the file more than once.
var ErrManyMatches = errors.New("more than one match")

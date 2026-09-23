package packs

import "errors"

// ErrInvalidTimeout is returned by the shell tool [ShellTools] registers when
// the call asks for a timeout outside the range the tool accepts.
var ErrInvalidTimeout = errors.New("invalid timeout")

// ErrInvalidRange is returned by the file tools [FileTools] registers when the
// call asks for a line offset, a line count or a match count below one.
var ErrInvalidRange = errors.New("invalid range")

// ErrNoMatch is returned by the file tool "file_edit" [FileTools] registers when
// the text the call asks to replace is not in the file.
var ErrNoMatch = errors.New("no match")

// ErrManyMatches is returned by the file tool "file_edit" [FileTools] registers
// when the text the call asks to replace is in the file more than once.
var ErrManyMatches = errors.New("more than one match")

// ErrInvalidPattern is returned by the file tool "file_search" [FileTools]
// registers when the call's regular expression, or its file name pattern,
// cannot be compiled.
var ErrInvalidPattern = errors.New("invalid pattern")

// ErrInvalidQuestion is returned by the decision tools [DecisionTools]
// registers when "decision_choice" is given fewer than two options or the same
// option twice, or "decision_score" fewer than two levels. The decision model is
// not asked.
var ErrInvalidQuestion = errors.New("invalid question")

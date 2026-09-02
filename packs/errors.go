package packs

import "errors"

// ErrInvalidTimeout is returned by the shell tool [ShellTools] registers when
// the call asks for a timeout outside the range the tool accepts.
var ErrInvalidTimeout = errors.New("invalid timeout")

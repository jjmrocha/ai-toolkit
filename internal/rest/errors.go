package rest

import "errors"

// ErrIdleTimeout is returned by a body wrapped with [WithIdleTimeout] when no
// data arrives within the timeout.
var ErrIdleTimeout = errors.New("idle timeout")

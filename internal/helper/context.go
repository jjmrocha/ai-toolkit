package helper

import (
	"context"
	"time"
)

// WithTimeout bounds ctx by timeout, but only when ctx carries no deadline of
// its own. An already-bounded ctx is returned unchanged, so a deadline the
// caller chose is never shortened by a default meant for callers that chose
// none. This is the difference from [context.WithTimeout], which always takes
// the earlier of the two.
//
// The returned [context.CancelFunc] must be called on either path, as with
// [context.WithTimeout].
func WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}

	return context.WithTimeout(ctx, timeout)
}

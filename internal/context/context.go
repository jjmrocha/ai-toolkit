package context

import (
	"context"
	"time"
)

// WithTimeout bounds ctx by timeout only when ctx has no deadline. A ctx with a
// deadline is returned unchanged, so a default never shortens a deadline the
// caller chose. [context.WithTimeout], by contrast, always takes the earlier
// one.
//
// Call the returned [context.CancelFunc] in both cases.
func WithTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}

	return context.WithTimeout(ctx, timeout)
}

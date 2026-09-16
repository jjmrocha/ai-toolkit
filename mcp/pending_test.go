package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testTimeout    = 50 * time.Millisecond
	testMargin     = 5 * time.Second
	testPollPeriod = time.Millisecond
)

func TestPendingRequestNewResettableTimeout(t *testing.T) {
	t.Run("returns a context that is still live", func(t *testing.T) {
		// given
		p := newPendingRequest()
		// when
		result := p.newResettableTimeout(context.Background(), "ait-1", testMargin)
		// then
		require.NotNil(t, result)
		assert.NoError(t, result.Err())
	})

	t.Run("cancels with the timeout cause once the budget expires", func(t *testing.T) {
		// given
		p := newPendingRequest()
		// when
		ctx := p.newResettableTimeout(context.Background(), "ait-1", testTimeout)
		// then
		<-ctx.Done()
		assert.ErrorIs(t, ctx.Err(), context.Canceled)
		assert.ErrorIs(t, context.Cause(ctx), ErrRequestTimeout)
	})

	t.Run("cancels with the parent when the parent is cancelled", func(t *testing.T) {
		// given
		p := newPendingRequest()
		parent, cancel := context.WithCancel(context.Background())
		ctx := p.newResettableTimeout(parent, "ait-1", testMargin)
		// when
		cancel()
		// then
		<-ctx.Done()
		assert.ErrorIs(t, ctx.Err(), context.Canceled)
	})

	t.Run("keeps each token on its own budget", func(t *testing.T) {
		// given
		p := newPendingRequest()
		// when
		first := p.newResettableTimeout(context.Background(), "ait-1", testTimeout)
		second := p.newResettableTimeout(context.Background(), "ait-2", testMargin)
		// then
		<-first.Done()
		assert.NoError(t, second.Err())
	})
}

func TestPendingRequestReset(t *testing.T) {
	t.Run("extends the budget of a call still in flight", func(t *testing.T) {
		// given
		p := newPendingRequest()
		ctx := p.newResettableTimeout(context.Background(), "ait-1", testTimeout)
		deadline := time.Now().Add(testTimeout)
		// when: progress keeps arriving until the original budget would have expired
		for time.Now().Before(deadline) {
			p.reset("ait-1")
			time.Sleep(testPollPeriod)
		}
		// then
		assert.NoError(t, ctx.Err())
	})

	t.Run("ignores a token nobody is waiting on", func(t *testing.T) {
		// given
		p := newPendingRequest()
		ctx := p.newResettableTimeout(context.Background(), "ait-1", testMargin)
		// when: progress arrives for a call that is not in flight
		p.reset("ait-99")
		// then
		assert.NoError(t, ctx.Err())
	})

	t.Run("ignores a token whose call already stopped", func(t *testing.T) {
		// given
		p := newPendingRequest()
		p.newResettableTimeout(context.Background(), "ait-1", testMargin)
		p.stop("ait-1")
		// when: a late progress notification arrives
		p.reset("ait-1")
		// then
		assert.Empty(t, p.requests)
	})
}

func TestPendingRequestStop(t *testing.T) {
	t.Run("cancels the context without the timeout cause", func(t *testing.T) {
		// given
		p := newPendingRequest()
		ctx := p.newResettableTimeout(context.Background(), "ait-1", testMargin)
		// when
		p.stop("ait-1")
		// then
		<-ctx.Done()
		assert.ErrorIs(t, ctx.Err(), context.Canceled)
		assert.ErrorIs(t, context.Cause(ctx), context.Canceled)
		assert.NotErrorIs(t, context.Cause(ctx), ErrRequestTimeout)
	})

	t.Run("forgets the token, so finished calls do not accumulate", func(t *testing.T) {
		// given
		p := newPendingRequest()
		p.newResettableTimeout(context.Background(), "ait-1", testMargin)
		p.newResettableTimeout(context.Background(), "ait-2", testMargin)
		// when
		p.stop("ait-1")
		p.stop("ait-2")
		// then
		assert.Empty(t, p.requests)
	})

	t.Run("ignores a token nobody is waiting on", func(t *testing.T) {
		// given
		p := newPendingRequest()
		ctx := p.newResettableTimeout(context.Background(), "ait-1", testMargin)
		// when
		p.stop("ait-99")
		// then
		assert.NoError(t, ctx.Err())
	})

	t.Run("is safe to call more than once", func(t *testing.T) {
		// given
		p := newPendingRequest()
		ctx := p.newResettableTimeout(context.Background(), "ait-1", testMargin)
		p.stop("ait-1")
		// when
		p.stop("ait-1")
		// then
		assert.ErrorIs(t, ctx.Err(), context.Canceled)
	})
}

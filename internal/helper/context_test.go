package helper

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithTimeout(t *testing.T) {
	t.Run("bounds a context that carries no deadline", func(t *testing.T) {
		// given
		ctx := context.Background()
		// when
		result, cancel := WithTimeout(ctx, time.Minute)
		defer cancel()
		// then
		deadline, ok := result.Deadline()
		require.True(t, ok)
		assert.WithinDuration(t, time.Now().Add(time.Minute), deadline, time.Second)
	})

	t.Run("leaves a shorter deadline alone", func(t *testing.T) {
		// given
		ctx, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		expected, _ := ctx.Deadline()
		// when
		result, cancel := WithTimeout(ctx, time.Minute)
		defer cancel()
		// then
		deadline, ok := result.Deadline()
		require.True(t, ok)
		assert.Equal(t, expected, deadline)
	})

	t.Run("leaves a longer deadline alone", func(t *testing.T) {
		// given
		ctx, stop := context.WithTimeout(context.Background(), time.Hour)
		defer stop()
		expected, _ := ctx.Deadline()
		// when
		result, cancel := WithTimeout(ctx, time.Minute)
		defer cancel()
		// then
		deadline, ok := result.Deadline()
		require.True(t, ok)
		assert.Equal(t, expected, deadline)
	})

	t.Run("cancels a context it bounded", func(t *testing.T) {
		// given
		result, cancel := WithTimeout(context.Background(), time.Minute)
		// when
		cancel()
		// then
		assert.ErrorIs(t, result.Err(), context.Canceled)
	})

	t.Run("does not cancel a context it left alone", func(t *testing.T) {
		// given
		ctx, stop := context.WithTimeout(context.Background(), time.Minute)
		defer stop()
		result, cancel := WithTimeout(ctx, time.Second)
		// when
		cancel()
		// then
		assert.NoError(t, result.Err())
	})
}

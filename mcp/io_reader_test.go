package mcp

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadBytes(t *testing.T) {
	t.Run("reads up to and including the first delimiter", func(t *testing.T) {
		// given
		r := strings.NewReader("hello\nworld\n")
		// when
		result, err := ReadBytes(t.Context(), r, '\n')
		// then
		require.NoError(t, err)
		assert.Equal(t, []byte("hello\n"), result)
	})

	t.Run("returns the partial line and io.EOF when no delimiter is found", func(t *testing.T) {
		// given
		r := strings.NewReader("partial")
		// when
		result, err := ReadBytes(t.Context(), r, '\n')
		// then
		assert.ErrorIs(t, err, io.EOF)
		assert.Equal(t, []byte("partial"), result)
	})

	t.Run("returns io.EOF for an empty reader", func(t *testing.T) {
		// given
		r := strings.NewReader("")
		// when
		result, err := ReadBytes(t.Context(), r, '\n')
		// then
		assert.ErrorIs(t, err, io.EOF)
		assert.Empty(t, result)
	})

	t.Run("returns the context error without reading when already cancelled", func(t *testing.T) {
		// given
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		// when
		result, err := ReadBytes(ctx, strings.NewReader("data\n"), '\n')
		// then
		assert.ErrorIs(t, err, context.Canceled)
		assert.Nil(t, result)
	})
}

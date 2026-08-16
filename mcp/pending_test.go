package mcp

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPendingRequestNewRequest(t *testing.T) {
	t.Run("returns a future that is not yet resolved", func(t *testing.T) {
		// given
		p := newPendingRequest()
		// when
		result := p.newRequest(1)
		// then
		require.NotNil(t, result)
		assert.False(t, result.Done())
	})

	t.Run("gives each id its own future", func(t *testing.T) {
		// given
		p := newPendingRequest()
		// when
		first := p.newRequest(1)
		second := p.newRequest(2)
		// then
		assert.NotSame(t, first, second)
	})
}

func TestPendingRequestResolve(t *testing.T) {
	t.Run("resolves the future waiting on that id", func(t *testing.T) {
		// given
		p := newPendingRequest()
		f := p.newRequest(1)
		// when
		p.resolve(1, map[string]any{"ok": true})
		// then
		result, err := f.Await()
		require.NoError(t, err)
		expected := map[string]any{"ok": true}
		assert.Equal(t, expected, result)
	})

	t.Run("ignores an id nobody is waiting on", func(t *testing.T) {
		// given
		p := newPendingRequest()
		f := p.newRequest(1)
		// when: a response arrives for a request that is not in flight
		p.resolve(99, map[string]any{"ok": true})
		// then
		assert.False(t, f.Done())
	})

	t.Run("forgets the id, so a repeat response is ignored", func(t *testing.T) {
		// given
		p := newPendingRequest()
		f := p.newRequest(1)
		p.resolve(1, map[string]any{"first": true})
		// when: the server answers the same id twice
		p.resolve(1, map[string]any{"second": true})
		// then: the first answer stands
		result, err := f.Await()
		require.NoError(t, err)
		expected := map[string]any{"first": true}
		assert.Equal(t, expected, result)
	})
}

func TestPendingRequestReject(t *testing.T) {
	t.Run("fails the future waiting on that id", func(t *testing.T) {
		// given
		p := newPendingRequest()
		f := p.newRequest(1)
		// when
		p.reject(1, ErrMCPConnectionClosed)
		// then
		result, err := f.Await()
		assert.ErrorIs(t, err, ErrMCPConnectionClosed)
		assert.Nil(t, result)
	})

	t.Run("ignores an id nobody is waiting on", func(t *testing.T) {
		// given
		p := newPendingRequest()
		f := p.newRequest(1)
		// when
		p.reject(99, ErrMCPConnectionClosed)
		// then
		assert.False(t, f.Done())
	})

	t.Run("leaves an already answered request untouched", func(t *testing.T) {
		// given: this is the cancel race — the caller gives up as the answer lands
		p := newPendingRequest()
		f := p.newRequest(1)
		p.resolve(1, map[string]any{"ok": true})
		// when: the abandoning caller rejects the same id afterwards
		p.reject(1, errors.New("too late"))
		// then: the answer that arrived first stands
		result, err := f.Await()
		require.NoError(t, err)
		expected := map[string]any{"ok": true}
		assert.Equal(t, expected, result)
	})
}

func TestPendingRequestFailAll(t *testing.T) {
	t.Run("fails every request still in flight", func(t *testing.T) {
		// given
		p := newPendingRequest()
		first := p.newRequest(1)
		second := p.newRequest(2)
		// when
		p.failAll(ErrMCPConnectionClosed)
		// then
		_, firstErr := first.Await()
		_, secondErr := second.Await()
		assert.ErrorIs(t, firstErr, ErrMCPConnectionClosed)
		assert.ErrorIs(t, secondErr, ErrMCPConnectionClosed)
	})

	t.Run("forgets every id, so a late response is ignored", func(t *testing.T) {
		// given
		p := newPendingRequest()
		f := p.newRequest(1)
		p.failAll(ErrMCPConnectionClosed)
		// when: the server answers after the connection is gone
		p.resolve(1, map[string]any{"late": true})
		// then: the failure stands
		result, err := f.Await()
		assert.ErrorIs(t, err, ErrMCPConnectionClosed)
		assert.Nil(t, result)
	})

	t.Run("is a no-op when nothing is in flight", func(t *testing.T) {
		// given
		p := newPendingRequest()
		// when
		p.failAll(ErrMCPConnectionClosed)
		// then: the tracker stays usable for the requests that follow
		result := p.newRequest(1)
		require.NotNil(t, result)
		assert.False(t, result.Done())
	})
}

func TestPendingRequestConcurrentAccess(t *testing.T) {
	// given: correctness here is enforced by the race detector
	const goroutines = 50
	p := newPendingRequest()

	var wg sync.WaitGroup
	// when
	for id := range goroutines {
		wg.Go(func() {
			f := p.newRequest(id)
			p.resolve(id, map[string]any{"id": id})
			// then
			result, err := f.Await()
			assert.NoError(t, err)
			assert.Equal(t, map[string]any{"id": id}, result)
		})
	}

	wg.Wait()
}

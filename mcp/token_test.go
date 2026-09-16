package mcp

import (
	"math"
	"sync"
	"testing"

	"github.com/jjmrocha/go-algo/sets"
	"github.com/stretchr/testify/assert"
)

func TestTokenNext(t *testing.T) {
	t.Run("starts at one", func(t *testing.T) {
		// given
		s := newToken()
		// when
		result := s.next()
		// then
		expected := "ait-1"
		assert.Equal(t, expected, result)
	})

	t.Run("increments on each call", func(t *testing.T) {
		// given
		s := newToken()
		s.next()
		s.next()
		// when
		result := s.next()
		// then
		expected := "ait-3"
		assert.Equal(t, expected, result)
	})

	t.Run("wraps back to one instead of overflowing", func(t *testing.T) {
		// given
		s := newToken()
		s.val.Store(math.MaxInt64)
		// when
		result := s.next()
		// then
		expected := "ait-1"
		assert.Equal(t, expected, result)
	})
}

func TestTokenConcurrentAccess(t *testing.T) {
	// given: tokens correlate progress notifications, so uniqueness is the property that matters
	const goroutines = 100
	s := newToken()
	results := make(chan string, goroutines)

	var wg sync.WaitGroup
	// when
	for range goroutines {
		wg.Go(func() {
			results <- s.next()
		})
	}

	wg.Wait()
	close(results)
	// then
	seen := sets.New[string]()
	for value := range results {
		seen.Add(value)
	}

	expected := goroutines
	assert.Equal(t, expected, seen.Len())
}

package mcp

import (
	"sync"
	"testing"

	"github.com/jjmrocha/go-algo/sets"
	"github.com/stretchr/testify/assert"
)

func TestSeqNumNext(t *testing.T) {
	t.Run("starts at one", func(t *testing.T) {
		// given
		s := newSeqNum()
		// when
		result := s.next()
		// then
		expected := 1
		assert.Equal(t, expected, result)
	})

	t.Run("increments on each call", func(t *testing.T) {
		// given
		s := newSeqNum()
		s.next()
		s.next()
		// when
		result := s.next()
		// then
		expected := 3
		assert.Equal(t, expected, result)
	})
}

func TestSeqNumConcurrentAccess(t *testing.T) {
	t.Run("hands out a distinct value to every caller", func(t *testing.T) {
		// given: ids collide silently, so uniqueness is the property that matters
		const goroutines = 100
		s := newSeqNum()
		results := make(chan int, goroutines)

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
		seen := sets.New[int]()
		for value := range results {
			seen.Add(value)
		}

		expected := goroutines
		assert.Equal(t, expected, seen.Len())
	})
}

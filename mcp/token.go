package mcp

import (
	"fmt"
	"math"
	"sync/atomic"
)

const format = "ait-%d"

type token struct {
	val atomic.Int64
}

func newToken() *token {
	return &token{}
}

func (s *token) next() string {
	for {
		currentValue := s.val.Load()
		newVal := currentValue + 1

		if currentValue == math.MaxInt64 {
			newVal = 1
		}

		if s.val.CompareAndSwap(currentValue, newVal) {
			return fmt.Sprintf(format, newVal)
		}
	}
}

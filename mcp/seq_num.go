package mcp

import "sync"

type seqNum struct {
	mu  sync.Mutex
	val int
}

func newSeqNum() *seqNum {
	return &seqNum{
		val: 0,
	}
}

func (s *seqNum) next() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.val++
	return s.val
}

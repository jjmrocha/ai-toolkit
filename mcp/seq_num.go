package mcp

import "sync/atomic"

type seqNum struct {
	val atomic.Int64
}

func newSeqNum() *seqNum {
	return &seqNum{}
}

func (s *seqNum) next() int {
	return int(s.val.Add(1))
}

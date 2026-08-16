package mcp

import (
	"sync"

	"github.com/jjmrocha/go-algo/future"
)

type pendingRequest struct {
	mu       sync.Mutex
	requests map[int]*future.Future[map[string]any]
}

func newPendingRequest() *pendingRequest {
	return &pendingRequest{
		requests: make(map[int]*future.Future[map[string]any]),
	}
}

func (p *pendingRequest) newRequest(id int) *future.Future[map[string]any] {
	p.mu.Lock()
	defer p.mu.Unlock()

	f := future.New[map[string]any]()
	p.requests[id] = f
	return f
}

func (p *pendingRequest) resolve(id int, result map[string]any) {
	p.settle(id, result, nil)
}

func (p *pendingRequest) reject(id int, err error) {
	p.settle(id, nil, err)
}

func (p *pendingRequest) settle(id int, result map[string]any, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if f, ok := p.requests[id]; ok {
		f.Resolve(result, err)
		delete(p.requests, id)
	}
}

func (p *pendingRequest) failAll(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, f := range p.requests {
		f.Resolve(nil, err)
	}

	clear(p.requests)
}

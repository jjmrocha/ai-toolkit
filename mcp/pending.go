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

func (p *pendingRequest) add(id int) *future.Future[map[string]any] {
	p.mu.Lock()
	defer p.mu.Unlock()

	f := future.New[map[string]any]()
	p.requests[id] = f
	return f
}

func (p *pendingRequest) resolve(id int, result map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if f, ok := p.requests[id]; ok {
		f.Resolve(result, nil)
		delete(p.requests, id)
	}
}

func (p *pendingRequest) reject(id int, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if f, ok := p.requests[id]; ok {
		f.Resolve(nil, err)
		delete(p.requests, id)
	}
}

func (p *pendingRequest) failAll(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, f := range p.requests {
		f.Resolve(nil, err)
	}

	p.requests = make(map[int]*future.Future[map[string]any])
}

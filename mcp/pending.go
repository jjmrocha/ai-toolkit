package mcp

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrRequestTimeout = errors.New("request timeout")

type resettableTimeout struct {
	mu      sync.Mutex
	cancel  context.CancelCauseFunc
	timer   *time.Timer
	timeout time.Duration
}

type pendingRequest struct {
	mu       sync.Mutex
	requests map[string]*resettableTimeout
}

func newPendingRequest() *pendingRequest {
	return &pendingRequest{
		requests: make(map[string]*resettableTimeout),
	}
}

func (p *pendingRequest) newResettableTimeout(parent context.Context, token string, d time.Duration) context.Context {
	p.mu.Lock()
	defer p.mu.Unlock()

	ctx, cancel := context.WithCancelCause(parent)

	r := resettableTimeout{
		cancel:  cancel,
		timeout: d,
	}
	p.requests[token] = &r

	r.timer = time.AfterFunc(d, func() {
		cancel(ErrRequestTimeout)
	})

	return ctx
}

func (p *pendingRequest) reset(token string) {
	p.mu.Lock()
	r, ok := p.requests[token]
	p.mu.Unlock()

	if !ok {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.timer.Stop() {
		return
	}

	r.timer.Reset(r.timeout)
}

func (p *pendingRequest) stop(token string) {
	p.mu.Lock()
	r, ok := p.requests[token]
	delete(p.requests, token)
	p.mu.Unlock()

	if !ok {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.timer.Stop()
	r.cancel(context.Canceled)
}

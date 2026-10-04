package pool

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrNotOpen is returned only when an existing-only read lease successfully
// inspected the pool and found no runtime for the requested brain.
var ErrNotOpen = errors.New("hosted runtime is not open")

// AcquireExistingForRead pins a warm runtime without opening, initializing,
// evicting, or flushing it. Unlike AcquireExisting, absence is distinguishable
// from capacity and shutdown so callers can choose a fenced cold-file read.
func (p *Pool) AcquireExistingForRead(ctx context.Context, id string) (*Runtime, func(), error) {
	if p == nil {
		return nil, nil, ErrCapacity
	}
	if !p.mu.TryLock() {
		return nil, nil, ErrCapacity
	}
	defer p.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if p.closed || p.inFlight >= p.cfg.MaxInFlight {
		return nil, nil, ErrCapacity
	}
	runtime := p.open[id]
	if runtime == nil {
		return nil, nil, ErrNotOpen
	}
	runtime.users++
	runtime.last = time.Now()
	p.inFlight++
	p.wg.Add(1)
	var once sync.Once
	release := func() {
		once.Do(func() {
			p.mu.Lock()
			runtime.users--
			runtime.last = time.Now()
			p.inFlight--
			p.mu.Unlock()
			p.wg.Done()
		})
	}
	return runtime, release, nil
}

package writer

import (
	"context"
	"sync"
)

// runLock serializes Render and Git publication while allowing a blocked
// Flush to stop waiting when its caller's context is canceled. Waiters sleep
// on changed, so cancellation does not require polling.
type runLock struct {
	mu      sync.Mutex
	held    bool
	waiters int
	changed chan struct{}
}

func newRunLock() runLock { return runLock{changed: make(chan struct{})} }

func (l *runLock) lockContext(ctx context.Context) error {
	if ctx == nil {
		return ErrNilCommitContext
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		l.mu.Lock()
		if !l.held {
			l.held = true
			l.mu.Unlock()
			return nil
		}
		l.waiters++
		changed := l.changedLocked()
		l.mu.Unlock()
		select {
		case <-ctx.Done():
			l.mu.Lock()
			l.waiters--
			l.mu.Unlock()
			return ctx.Err()
		case <-changed:
			l.mu.Lock()
			l.waiters--
			l.mu.Unlock()
		}
	}
}

func (l *runLock) changedLocked() chan struct{} {
	if l.changed == nil {
		l.changed = make(chan struct{})
	}
	return l.changed
}

func (l *runLock) unlock() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.held {
		return
	}
	l.held = false
	close(l.changedLocked())
	l.changed = make(chan struct{})
}

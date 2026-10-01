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
		changed := l.changedLocked()
		l.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

func (l *runLock) tryLock() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held {
		return false
	}
	l.held = true
	return true
}

func (l *runLock) changedLocked() chan struct{} {
	if l.changed == nil {
		l.changed = make(chan struct{})
	}
	return l.changed
}

// changedChannel snapshots the current release notification. A release
// racing with the subsequent wait closes this exact channel, so no wakeup is
// lost.
func (l *runLock) changedChannel() <-chan struct{} {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.changedLocked()
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

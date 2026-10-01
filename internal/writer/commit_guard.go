package writer

import (
	"context"
	"errors"
	"sync"
)

var (
	// ErrNilCommitContext indicates that a context-aware commit operation was
	// called without a context.
	ErrNilCommitContext = errors.New("writer: nil commit context")
	// ErrNilCommitFence indicates that no checker callback was supplied.
	ErrNilCommitFence = errors.New("writer: nil commit fence callback")
)

// commitGate is a context-aware, writer-preferring read/write gate. The
// exclusive side is used only by canonical-state checkers; queue jobs that
// call an external provider must not hold the shared side.
type commitGate struct {
	mu            sync.Mutex
	readers       int
	writer        bool
	waitingWriter int
	changed       chan struct{}
}

func (g *commitGate) changedLocked() {
	if g.changed == nil {
		g.changed = make(chan struct{})
		return
	}
	close(g.changed)
	g.changed = make(chan struct{})
}

func (g *commitGate) enterShared(ctx context.Context) (func(), error) {
	if ctx == nil {
		return nil, ErrNilCommitContext
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		g.mu.Lock()
		if !g.writer && g.waitingWriter == 0 {
			g.readers++
			g.mu.Unlock()
			var once sync.Once
			return func() {
				once.Do(func() {
					g.mu.Lock()
					g.readers--
					g.changedLocked()
					g.mu.Unlock()
				})
			}, nil
		}
		changed := g.changed
		if changed == nil {
			g.changed = make(chan struct{})
			changed = g.changed
		}
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-changed:
		}
	}
}

// tryEnterShared never waits. On denial it returns the gate-change channel
// observed under the gate lock; callers can release unrelated locks before
// waiting on that channel without missing a transition.
func (g *commitGate) tryEnterShared(ctx context.Context) (leave func(), changed <-chan struct{}, err error) {
	if ctx == nil {
		return nil, nil, ErrNilCommitContext
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.writer && g.waitingWriter == 0 {
		g.readers++
		var once sync.Once
		return func() {
			once.Do(func() {
				g.mu.Lock()
				g.readers--
				g.changedLocked()
				g.mu.Unlock()
			})
		}, nil, nil
	}
	if g.changed == nil {
		g.changed = make(chan struct{})
	}
	return nil, g.changed, nil
}

func (g *commitGate) enterExclusive(ctx context.Context) (func(), error) {
	if ctx == nil {
		return nil, ErrNilCommitContext
	}
	g.mu.Lock()
	g.waitingWriter++
	g.changedLocked()
	g.mu.Unlock()

	for {
		if err := ctx.Err(); err != nil {
			g.mu.Lock()
			g.waitingWriter--
			g.changedLocked()
			g.mu.Unlock()
			return nil, err
		}
		g.mu.Lock()
		if !g.writer && g.readers == 0 {
			g.waitingWriter--
			g.writer = true
			g.changedLocked()
			g.mu.Unlock()
			var once sync.Once
			return func() {
				once.Do(func() {
					g.mu.Lock()
					g.writer = false
					g.changedLocked()
					g.mu.Unlock()
				})
			}, nil
		}
		changed := g.changed
		if changed == nil {
			g.changed = make(chan struct{})
			changed = g.changed
		}
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			g.mu.Lock()
			g.waitingWriter--
			g.changedLocked()
			g.mu.Unlock()
			return nil, ctx.Err()
		case <-changed:
		}
	}
}

// EnterCommit enters the shared canonical-write section. Callers must invoke
// the returned release function exactly once, after source mutation and its
// durability boundary are complete.
func (q *Queue) EnterCommit(ctx context.Context) (leave func(), err error) {
	return q.commit.enterShared(ctx)
}

// AcquireCommitFence enters the exclusive canonical-check section. Release
// the returned function exactly once after the checker and ledger transition
// are complete. The checker must not call queue methods or wait for runLock.
func (q *Queue) AcquireCommitFence(ctx context.Context) (release func(), err error) {
	return q.commit.enterExclusive(ctx)
}

// WithCommitFence runs check while no shared canonical-write section is
// active. checkErr is the checker's outcome; acquireErr reports failure to
// acquire the exclusive section (including context cancellation while
// waiting), so callers can distinguish those cases. The callback must only
// inspect canonical state and update its ledger; it must not call Submit or
// Flush, or wait for runMu, because the queue's shared writer order is
// commit-section then runMu.
func (q *Queue) WithCommitFence(ctx context.Context, check func(context.Context) error) (checkErr error, acquireErr error) {
	if check == nil {
		return nil, ErrNilCommitFence
	}
	leave, err := q.commit.enterExclusive(ctx)
	if err != nil {
		return nil, err
	}
	defer leave()
	return check(ctx), nil
}

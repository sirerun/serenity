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

// WithCommitFence runs check while no shared canonical-write section is
// active. checkErr is the checker's outcome; acquireErr reports failure to
// acquire the exclusive section (including context cancellation while
// waiting), so callers can distinguish those cases.
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

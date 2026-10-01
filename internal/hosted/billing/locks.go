package billing

import (
	"context"
	"sync"
)

// keyedLocks serializes operations that touch the same billing account while
// allowing unrelated accounts to make progress independently. Its mutex only
// protects registry bookkeeping; callers never perform provider work under it.
type keyedLocks struct {
	mu      sync.Mutex
	entries map[string]*keyedLock
}

type keyedLock struct {
	token chan struct{}
	refs  int
}

func (l *keyedLocks) acquire(ctx context.Context, key string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.mu.Lock()
	if l.entries == nil {
		l.entries = make(map[string]*keyedLock)
	}
	entry := l.entries[key]
	if entry == nil {
		entry = &keyedLock{token: make(chan struct{}, 1)}
		entry.token <- struct{}{}
		l.entries[key] = entry
	}
	entry.refs++
	l.mu.Unlock()

	select {
	case <-entry.token:
		if err := ctx.Err(); err != nil {
			entry.token <- struct{}{}
			l.dropRef(key, entry)
			return nil, err
		}
		var once sync.Once
		return func() {
			once.Do(func() {
				entry.token <- struct{}{}
				l.dropRef(key, entry)
			})
		}, nil
	case <-ctx.Done():
		l.dropRef(key, entry)
		return nil, ctx.Err()
	}
}

func (l *keyedLocks) dropRef(key string, entry *keyedLock) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry.refs--
	if entry.refs == 0 && l.entries[key] == entry {
		delete(l.entries, key)
	}
}

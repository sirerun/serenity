package contractstest

import (
	"context"
	"sync"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

// MemBrainFence is the reference contracts.BrainFence: a per-brain
// readers-writer section that is writer-preferring (a waiting Fence blocks new
// commit sections so it cannot starve) and context-aware (a Fence whose ctx
// ends while a section is open returns ErrBrainNotQuiescent and holds nothing).
type MemBrainFence struct {
	mu     sync.Mutex
	brains map[string]*fenceState
}

type fenceState struct {
	active  int
	fenced  bool
	waiting int
	changed chan struct{}
}

func NewMemBrainFence() *MemBrainFence { return &MemBrainFence{brains: map[string]*fenceState{}} }

func (f *MemBrainFence) state(brain string) *fenceState {
	st, ok := f.brains[brain]
	if !ok {
		st = &fenceState{changed: make(chan struct{})}
		f.brains[brain] = st
	}
	return st
}

// notify wakes every waiter; the caller holds f.mu.
func (st *fenceState) notify() {
	close(st.changed)
	st.changed = make(chan struct{})
}

func (f *MemBrainFence) EnterCommit(ctx context.Context, brain string) (func(), error) {
	for {
		f.mu.Lock()
		st := f.state(brain)
		if !st.fenced && st.waiting == 0 {
			st.active++
			f.mu.Unlock()
			var once sync.Once
			return func() {
				once.Do(func() {
					f.mu.Lock()
					st.active--
					st.notify()
					f.mu.Unlock()
				})
			}, nil
		}
		ch := st.changed
		f.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (f *MemBrainFence) Fence(ctx context.Context, brain string) (func(), error) {
	f.mu.Lock()
	st := f.state(brain)
	st.waiting++
	f.mu.Unlock()
	for {
		f.mu.Lock()
		if !st.fenced && st.active == 0 {
			st.waiting--
			st.fenced = true
			f.mu.Unlock()
			var once sync.Once
			return func() {
				once.Do(func() {
					f.mu.Lock()
					st.fenced = false
					st.notify()
					f.mu.Unlock()
				})
			}, nil
		}
		ch := st.changed
		f.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			f.mu.Lock()
			st.waiting--
			st.notify()
			f.mu.Unlock()
			return nil, contracts.ErrBrainNotQuiescent
		}
	}
}

// NoFence is a deliberately unsafe contracts.BrainFence that excludes nothing.
// It exists only as a negative control: it is what a reconciler that trusts
// lease expiry alone effectively has, and the suite uses it to show the
// resulting uncharged-write loss really occurs.
type NoFence struct{}

func (NoFence) EnterCommit(context.Context, string) (func(), error) { return func() {}, nil }
func (NoFence) Fence(context.Context, string) (func(), error)       { return func() {}, nil }

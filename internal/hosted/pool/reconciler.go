package pool

import (
	"context"
	"errors"
	"sync"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

// Reconciler binds an exclusive queue fence and its canonical check to one
// leased Runtime. The caller must keep the release returned by Fence until its
// ledger transition has completed.
type Reconciler struct {
	pool         *Pool
	existingOnly bool

	mu       sync.Mutex
	starting map[string]struct{}
	held     map[string]*heldFence
}

type heldFence struct {
	runtime      *Runtime
	releaseQueue func()
	releasePool  func()
}

// NewReconciler creates a conservative pool-backed BrainFence and
// CanonicalChecker. A nil pool is retained as unavailable and fails closed.
func NewReconciler(p *Pool) *Reconciler {
	return &Reconciler{
		pool:     p,
		starting: make(map[string]struct{}),
		held:     make(map[string]*heldFence),
	}
}

// NewExistingReconciler never opens a cold runtime. It is safe to use while
// deletion can Drop and remove brains: a missing runtime remains deferred.
func NewExistingReconciler(p *Pool) *Reconciler {
	r := NewReconciler(p)
	r.existingOnly = true
	return r
}

func (r *Reconciler) acquirePool(ctx context.Context, brainID string) (*Runtime, func(), error) {
	if r.existingOnly {
		return r.pool.AcquireExisting(ctx, brainID)
	}
	return r.pool.Acquire(ctx, brainID)
}

// EnterCommit acquires the Runtime matching brainID and opens its shared
// queue commit section. Both resources remain leased until leave is called.
func (r *Reconciler) EnterCommit(ctx context.Context, brainID string) (func(), error) {
	if ctx == nil {
		return nil, errors.Join(contracts.ErrBrainNotQuiescent, errors.New("hosted pool: nil commit context"))
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(contracts.ErrBrainNotQuiescent, err)
	}
	if r == nil || r.pool == nil || brainID == "" {
		return nil, contracts.ErrBrainNotQuiescent
	}
	runtime, releasePool, err := r.acquirePool(ctx, brainID)
	if err != nil {
		if errors.Is(err, ErrCapacity) {
			return nil, errors.Join(contracts.ErrBrainNotQuiescent, err, ctx.Err())
		}
		return nil, err
	}
	leaveQueue, err := runtime.EnterCommit(ctx, brainID)
	if err != nil {
		releasePool()
		return nil, errors.Join(contracts.ErrBrainNotQuiescent, err, ctx.Err())
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			leaveQueue()
			releasePool()
		})
	}, nil
}

// Fence acquires a pool lease before opening the Runtime's exclusive commit
// fence. That lease prevents eviction, Drop, and Close from removing the
// checked Runtime before the caller settles its ledger transition and releases
// the fence.
func (r *Reconciler) Fence(ctx context.Context, brainID string) (func(), error) {
	if ctx == nil {
		return nil, errors.Join(contracts.ErrBrainNotQuiescent, errors.New("hosted pool: nil reconciliation context"))
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(contracts.ErrBrainNotQuiescent, err)
	}
	if r == nil || r.pool == nil || brainID == "" {
		return nil, contracts.ErrBrainNotQuiescent
	}

	r.mu.Lock()
	if _, exists := r.starting[brainID]; exists {
		r.mu.Unlock()
		return nil, contracts.ErrBrainNotQuiescent
	}
	if r.held[brainID] != nil {
		r.mu.Unlock()
		return nil, contracts.ErrBrainNotQuiescent
	}
	r.starting[brainID] = struct{}{}
	r.mu.Unlock()

	clearStarting := func() {
		r.mu.Lock()
		delete(r.starting, brainID)
		r.mu.Unlock()
	}

	runtime, releasePool, err := r.acquirePool(ctx, brainID)
	if err != nil {
		clearStarting()
		if errors.Is(err, ErrCapacity) {
			return nil, errors.Join(contracts.ErrBrainNotQuiescent, err, ctx.Err())
		}
		return nil, err
	}

	releaseQueue, err := runtime.Fence(ctx, brainID)
	if err != nil {
		releasePool()
		clearStarting()
		return nil, errors.Join(contracts.ErrBrainNotQuiescent, err, ctx.Err())
	}
	if err = ctx.Err(); err != nil {
		releaseQueue()
		releasePool()
		clearStarting()
		return nil, errors.Join(contracts.ErrBrainNotQuiescent, err)
	}

	lease := &heldFence{runtime: runtime, releaseQueue: releaseQueue, releasePool: releasePool}
	r.mu.Lock()
	delete(r.starting, brainID)
	r.held[brainID] = lease
	r.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			if r.held[brainID] == lease {
				delete(r.held, brainID)
			}
			lease.releaseQueue()
			r.mu.Unlock()
			lease.releasePool()
		})
	}, nil
}

// Check delegates only through the exact Runtime whose queue fence is still
// held. It holds the adapter mutex across the check so the returned release
// cannot drop that fence or its pool lease during canonical inspection.
func (r *Reconciler) Check(ctx context.Context, rec contracts.OperationRecord) (contracts.CanonicalVerdict, error) {
	unknown := contracts.CanonicalVerdict{Outcome: contracts.CanonicalUnknown, Ref: "brain_fence_not_held"}
	if ctx == nil {
		return unknown, errors.New("hosted pool: nil canonical-check context")
	}
	if err := ctx.Err(); err != nil {
		return unknown, err
	}
	if r == nil {
		return unknown, contracts.ErrBrainNotQuiescent
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	lease := r.held[rec.BrainID]
	if lease == nil || lease.runtime == nil || lease.runtime.brainID != rec.BrainID {
		return unknown, contracts.ErrBrainNotQuiescent
	}
	return lease.runtime.Check(ctx, rec)
}

var _ contracts.BrainFence = (*Reconciler)(nil)
var _ contracts.CanonicalChecker = (*Reconciler)(nil)

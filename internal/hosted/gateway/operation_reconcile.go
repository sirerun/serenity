package gateway

import (
	"context"
	"errors"
	"sync"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/pool"
)

// OperationReconciler retains the maintenance read fence while its pool
// adapter keeps the same runtime leased through proof and ledger transition.
type OperationReconciler struct {
	gateway      *Gateway
	brainsRoot   string
	runtimeFence contracts.BrainFence
	checker      contracts.CanonicalChecker
}

// NewOperationReconciler uses only existing runtimes and never takes an
// account lock. A concurrent deletion either sees the held lease or removes
// the runtime first, in which case reconciliation defers without reopening it.
func NewOperationReconciler(g *Gateway) *OperationReconciler {
	return newOperationReconciler(g, false)
}

// NewStartupOperationReconciler is restricted to private service assembly,
// before any handler or worker can access the new pool. Only this quiescent
// startup phase may open cold runtimes for conservative proof.
func NewStartupOperationReconciler(g *Gateway) *OperationReconciler {
	return newOperationReconciler(g, true)
}

func newOperationReconciler(g *Gateway, startup bool) *OperationReconciler {
	if g == nil {
		return &OperationReconciler{}
	}
	r := pool.NewExistingReconciler(g.Pool)
	if startup {
		r = pool.NewReconciler(g.Pool)
	}
	return &OperationReconciler{gateway: g, brainsRoot: g.Pool.BrainsRoot(), runtimeFence: r, checker: r}
}

func (r *OperationReconciler) EnterCommit(ctx context.Context, brainID string) (func(), error) {
	if r.runtimeFence == nil {
		return nil, contracts.ErrBrainNotQuiescent
	}
	// The writer already owns its maintenance/account admission; do not nest
	// those locks while entering the exact runtime's shared queue section.
	return r.runtimeFence.EnterCommit(ctx, brainID)
}

func (r *OperationReconciler) Fence(ctx context.Context, brainID string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	g := r.gateway
	if g == nil || g.Issuer == nil || g.Issuer.Store == nil || r.runtimeFence == nil {
		return nil, contracts.ErrBrainNotQuiescent
	}
	g.mu.Lock()
	closed := g.closed
	g.mu.Unlock()
	if closed || !g.Maintenance.TryRLock() {
		return nil, contracts.ErrBrainNotQuiescent
	}
	held := true
	defer func() {
		if held {
			g.Maintenance.RUnlock()
		}
	}()
	var path, state, status string
	if err := g.Issuer.Store.DB().QueryRowContext(ctx, "SELECT b.path_key,b.state,a.status FROM brains b JOIN accounts a ON a.id=b.account_id WHERE b.id=?", brainID).Scan(&path, &state, &status); err != nil {
		return nil, errors.Join(contracts.ErrBrainNotQuiescent, err)
	}
	if path != brainID || !validBrainPathKey(path) || state != "ready" || status != "active" || r.brainsRoot == "" {
		return nil, contracts.ErrBrainNotQuiescent
	}
	if _, present, err := validateBrainTree(ctx, r.brainsRoot, brainID); err != nil {
		return nil, err
	} else if !present {
		return nil, contracts.ErrBrainNotQuiescent
	}
	leave, err := r.runtimeFence.Fence(ctx, brainID)
	if err != nil {
		return nil, err
	}
	held = false
	var once sync.Once
	return func() { once.Do(func() { leave(); g.Maintenance.RUnlock() }) }, nil
}

func (r *OperationReconciler) Check(ctx context.Context, rec contracts.OperationRecord) (contracts.CanonicalVerdict, error) {
	if r.checker == nil || r.gateway == nil || r.gateway.Issuer == nil || r.gateway.Issuer.Store == nil {
		return contracts.CanonicalVerdict{}, contracts.ErrBrainNotQuiescent
	}
	var matches bool
	if err := r.gateway.Issuer.Store.DB().QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM brains b JOIN accounts a ON a.id=b.account_id WHERE b.id=? AND b.account_id=? AND b.path_key=? AND b.state='ready' AND a.status='active')", rec.BrainID, rec.AccountID, rec.BrainID).Scan(&matches); err != nil {
		return contracts.CanonicalVerdict{}, err
	}
	if !matches {
		return contracts.CanonicalVerdict{}, contracts.ErrBrainNotQuiescent
	}
	return r.checker.Check(ctx, rec)
}

var _ contracts.BrainFence = (*OperationReconciler)(nil)
var _ contracts.CanonicalChecker = (*OperationReconciler)(nil)

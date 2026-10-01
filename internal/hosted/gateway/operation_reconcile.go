package gateway

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/pool"
)

// OperationReconciler retains maintenance and account fences while its pool
// adapter keeps the same runtime leased through proof and ledger transition.
type OperationReconciler struct {
	gateway      *Gateway
	brainsRoot   string
	runtimeFence contracts.BrainFence
	checker      contracts.CanonicalChecker
}

func NewOperationReconciler(g *Gateway) *OperationReconciler {
	if g == nil {
		return &OperationReconciler{}
	}
	r := pool.NewReconciler(g.Pool)
	return &OperationReconciler{gateway: g, brainsRoot: g.Pool.BrainsRoot(), runtimeFence: r, checker: r}
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
	if closed {
		return nil, contracts.ErrBrainNotQuiescent
	}
	if !g.Maintenance.TryRLock() {
		return nil, contracts.ErrBrainNotQuiescent
	}
	held := true
	defer func() {
		if held {
			g.Maintenance.RUnlock()
		}
	}()
	var account, path, state string
	if err := g.Issuer.Store.DB().QueryRowContext(ctx, "SELECT account_id,path_key,state FROM brains WHERE id=?", brainID).Scan(&account, &path, &state); err != nil {
		return nil, errors.Join(contracts.ErrBrainNotQuiescent, err)
	}
	hash := sha256.Sum256([]byte(account))
	lock := &g.accountLocks[int(hash[0])%len(g.accountLocks)]
	if !lock.TryLock() {
		return nil, contracts.ErrBrainNotQuiescent
	}
	accountHeld := true
	defer func() {
		if accountHeld {
			lock.Unlock()
		}
	}()
	// Recheck under the same account lock used by deletion and live requests.
	var status string
	if err := g.Issuer.Store.DB().QueryRowContext(ctx, "SELECT b.path_key,b.state,a.status FROM brains b JOIN accounts a ON a.id=b.account_id WHERE b.id=? AND b.account_id=?", brainID, account).Scan(&path, &state, &status); err != nil {
		return nil, errors.Join(contracts.ErrBrainNotQuiescent, err)
	}
	if path != brainID || !validBrainPathKey(path) || state != "ready" || status != "active" {
		return nil, contracts.ErrBrainNotQuiescent
	}
	if r.brainsRoot == "" {
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
	accountHeld = false
	var once sync.Once
	return func() { once.Do(func() { leave(); lock.Unlock(); g.Maintenance.RUnlock() }) }, nil
}

func (r *OperationReconciler) Check(ctx context.Context, rec contracts.OperationRecord) (contracts.CanonicalVerdict, error) {
	if r.checker == nil || r.gateway == nil || r.gateway.Issuer == nil || r.gateway.Issuer.Store == nil {
		return contracts.CanonicalVerdict{}, contracts.ErrBrainNotQuiescent
	}
	var matches bool
	if err := r.gateway.Issuer.Store.DB().QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM brains WHERE id=? AND account_id=? AND path_key=?)", rec.BrainID, rec.AccountID, rec.BrainID).Scan(&matches); err != nil {
		return contracts.CanonicalVerdict{}, err
	}
	if !matches {
		return contracts.CanonicalVerdict{}, contracts.ErrBrainNotQuiescent
	}
	return r.checker.Check(ctx, rec)
}

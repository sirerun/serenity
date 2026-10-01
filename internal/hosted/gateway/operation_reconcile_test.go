package gateway

import (
	"context"
	"crypto/sha256"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/store"
)

type reconciliationFenceProbe struct {
	calls    int
	released int
}

func (p *reconciliationFenceProbe) Fence(context.Context, string) (func(), error) {
	p.calls++
	return func() { p.released++ }, nil
}
func TestOperationReconciliationHoldsLifecycleLocks(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	account, err := db.CreateAccount(ctx, "fence@example.test")
	if err != nil {
		t.Fatal(err)
	}
	brain := store.ID()
	if _, err = db.InsertBrain(ctx, account.ID, brain, brain, "ready", time.Now()); err != nil {
		t.Fatal(err)
	}
	g := &Gateway{Issuer: &credential.Issuer{Store: db}}
	probe := &reconciliationFenceProbe{}
	r := &OperationReconciler{gateway: g, runtimeFence: probe}
	g.Maintenance.Lock()
	if _, err = r.Fence(ctx, brain); !errors.Is(err, contracts.ErrBrainNotQuiescent) {
		t.Fatalf("backup fence bypassed: %v", err)
	}
	g.Maintenance.Unlock()
	leave, err := r.Fence(ctx, brain)
	if err != nil {
		t.Fatal(err)
	}
	if g.Maintenance.TryLock() {
		g.Maintenance.Unlock()
		t.Fatal("maintenance fence not retained")
	}
	hash := sha256.Sum256([]byte(account.ID))
	lock := &g.accountLocks[int(hash[0])%len(g.accountLocks)]
	if lock.TryLock() {
		lock.Unlock()
		t.Fatal("account lock not retained")
	}
	leave()
	leave()
	if probe.calls != 1 || probe.released != 1 {
		t.Fatalf("delegate counts %+v", probe)
	}
	if !g.Maintenance.TryLock() {
		t.Fatal("maintenance lock leaked")
	}
	g.Maintenance.Unlock()
	if !lock.TryLock() {
		t.Fatal("account lock leaked")
	}
	lock.Unlock()
	for _, status := range []string{"restore_pending", "deleting", "deleted"} {
		if _, err = db.DB().Exec("UPDATE accounts SET status=? WHERE id=?", status, account.ID); err != nil {
			t.Fatal(err)
		}
		if _, err = r.Fence(ctx, brain); !errors.Is(err, contracts.ErrBrainNotQuiescent) {
			t.Fatalf("opened frozen %s: %v", status, err)
		}
	}
	if probe.calls != 1 {
		t.Fatal("frozen accounts reached runtime")
	}
}

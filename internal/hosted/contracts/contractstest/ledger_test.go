package contractstest_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/contracts/contractstest"
)

func TestReferenceLedgerConformance(t *testing.T) {
	contractstest.RunLedgerSuite(t, contractstest.ReferenceLedger)
}

// A transition that aborts midway must leave neither counter moved and the row
// still reserved: the all-or-nothing half of "both counters finalize in one
// transaction". Specific to the reference model, because it needs the fault hook.
func TestReferenceFinalizeIsAtomicAcrossCounters(t *testing.T) {
	ctx := context.Background()
	clock := contractstest.NewClock()
	m := contractstest.NewMemLedger(clock.Now)
	rec, err := m.Reserve(ctx, contracts.ReserveRequest{AccountID: "a", BrainID: "b", QuotaPeriod: "2026-09", Source: "gateway.remember", LeaseFor: 1,
		Deltas: []contracts.ReserveDelta{{Metric: "writes", Units: 1, Limit: 10}, {Metric: "input_tokens", Units: 9, Limit: 100}}})
	if err != nil {
		t.Fatal(err)
	}
	boom := errors.New("crash between counters")
	m.FaultBeforeApply = func() error { return boom }
	ev := contracts.Evidence{Kind: contracts.EvidenceCommitted, Ref: "commit-1"}
	if _, err := m.Finalize(ctx, rec.ID, contracts.OperationCommitted, ev); !errors.Is(err, boom) {
		t.Fatalf("finalize err = %v, want the injected fault", err)
	}
	if m.Used("a", "2026-09", "writes") != 0 || m.Used("a", "2026-09", "input_tokens") != 0 {
		t.Fatal("a faulted finalize moved a counter")
	}
	got, _ := m.Get(ctx, rec.ID)
	if got.Phase != contracts.OperationReserved {
		t.Fatalf("faulted finalize left phase %s, want reserved", got.Phase)
	}
	m.FaultBeforeApply = nil
	if _, err := m.Finalize(ctx, rec.ID, contracts.OperationCommitted, ev); err != nil {
		t.Fatal(err)
	}
	if m.Used("a", "2026-09", "writes") != 1 || m.Used("a", "2026-09", "input_tokens") != 9 {
		t.Fatal("retry after the fault did not apply both counters")
	}
}

func TestBrainFenceExcludesAndDoesNotStarve(t *testing.T) {
	ctx := context.Background()
	f := contractstest.NewMemBrainFence()
	leave, err := f.EnterCommit(ctx, "b1")
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := f.Fence(cancelled, "b1"); !errors.Is(err, contracts.ErrBrainNotQuiescent) {
		t.Fatalf("fence with an open section = %v, want ErrBrainNotQuiescent", err)
	}
	// A different brain is independent.
	release, err := f.Fence(ctx, "b2")
	if err != nil {
		t.Fatalf("fence on an unrelated brain: %v", err)
	}
	release()
	// A refused Fence must not leave a phantom waiter blocking new sections.
	leave2, err := f.EnterCommit(ctx, "b1")
	if err != nil {
		t.Fatalf("section after a refused fence: %v", err)
	}
	leave2()
	leave()
	release, err = f.Fence(ctx, "b1")
	if err != nil {
		t.Fatalf("fence once every section left: %v", err)
	}
	// While fenced, a section cannot open.
	cancelled2, cancel2 := context.WithCancel(ctx)
	cancel2()
	if _, err := f.EnterCommit(cancelled2, "b1"); err == nil {
		t.Fatal("a commit section opened while the brain was fenced")
	}
	release()
	release() // idempotent
	if l, err := f.EnterCommit(ctx, "b1"); err != nil {
		t.Fatal(err)
	} else {
		l()
	}
}

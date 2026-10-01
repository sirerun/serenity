package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/operation"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func reserveInterruptedForTest(t *testing.T, db *store.Store, account, brain string) contracts.OperationRecord {
	t.Helper()
	ledger := &operation.Ledger{Store: db, Clock: func() time.Time { return time.Unix(100, 0).UTC() }}
	rec, err := ledger.Reserve(context.Background(), contracts.ReserveRequest{AccountID: account, BrainID: brain, ClientKey: "interrupted", Fingerprint: "test-fingerprint", QuotaPeriod: "2026-10", Source: "gateway.remember", LeaseFor: time.Second, Deltas: []contracts.ReserveDelta{{Metric: "writes", Units: 1, Limit: 2}, {Metric: "input_tokens", Units: 7, Limit: 20}}})
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestStartupReconcilesUnknownWithoutReleasingCapacity(t *testing.T) {
	db, cfg, account, path := deletionFixture(t)
	rec := reserveInterruptedForTest(t, db, account, filepath.Base(path))
	svc, err := assembleForTest(t, cfg, true, db, nil, deletionEmbedding{})
	if err != nil {
		t.Fatal(err)
	}
	if svc.operations != svc.Gateway.Operations {
		t.Fatal("startup and requests use different ledgers")
	}
	var phase string
	if err = db.DB().QueryRow("SELECT phase FROM operations WHERE id=?", rec.ID).Scan(&phase); err != nil {
		t.Fatal(err)
	}
	if phase != string(contracts.OperationPendingReview) {
		t.Fatalf("interrupted row phase %s", phase)
	}
	for _, metric := range []string{"writes", "input_tokens"} {
		var committed int64
		if err = db.DB().QueryRow("SELECT COALESCE((SELECT committed FROM usage_windows WHERE account_id=? AND window_key=? AND metric=?),0)", account, "2026-10", metric).Scan(&committed); err != nil {
			t.Fatal(err)
		}
		if committed != 0 {
			t.Fatalf("unknown %s charged %d", metric, committed)
		}
	}
	// Pending review is intentionally not eligible for automatic settlement.
	report, err := svc.reconcileOperations(context.Background())
	if err != nil || report.Committed != 0 || report.Released != 0 || report.PendingReview != 0 {
		t.Fatalf("operator row automatically reconciled: %+v %v", report, err)
	}
	if _, err = svc.operations.Reserve(context.Background(), contracts.ReserveRequest{AccountID: account, BrainID: filepath.Base(path), ClientKey: "independent", Fingerprint: "other", QuotaPeriod: "2026-10", Source: "gateway.remember", LeaseFor: time.Minute, Deltas: []contracts.ReserveDelta{{Metric: "writes", Units: 2, Limit: 2}}}); !errors.Is(err, contracts.ErrOperationLimitExceeded) {
		t.Fatalf("unknown hold freed capacity: %v", err)
	}
}

func TestOperationReconcileTickAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time)
	called := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		runOperationReconcileLoop(ctx, ticks, func(context.Context) error { close(called); return nil })
	}()
	ticks <- time.Now()
	<-called
	cancel()
	<-done
}

func TestServiceCloseJoinsOperationWorker(t *testing.T) {
	db, cfg, _, _ := deletionFixture(t)
	svc, err := assembleForTest(t, cfg, true, db, nil, deletionEmbedding{})
	if err != nil {
		t.Fatal(err)
	}
	done := svc.operationWorkerDone
	if done == nil {
		t.Fatal("operation worker not started")
	}
	if err = svc.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	default:
		t.Fatal("Close returned before worker joined")
	}
}

func TestTickerReconciliationDefersColdRuntimeWithoutInitializingIt(t *testing.T) {
	db, cfg, account, path := deletionFixture(t)
	svc, err := assembleForTest(t, cfg, true, db, nil, deletionEmbedding{})
	if err != nil {
		t.Fatal(err)
	}
	rec := reserveInterruptedForTest(t, db, account, filepath.Base(path))
	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time)
	reports := make(chan contracts.ReconcileReport, 1)
	errs := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		runOperationReconcileLoop(ctx, ticks, func(ctx context.Context) error {
			report, err := svc.reconcileOperations(ctx)
			reports <- report
			errs <- err
			return err
		})
	}()
	ticks <- time.Now()
	report := <-reports
	if err = <-errs; err != nil {
		cancel()
		<-done
		t.Fatal(err)
	}
	cancel()
	<-done
	if report.Deferred != 1 || report.Committed != 0 || report.Released != 0 {
		t.Fatalf("cold ticker report %+v", report)
	}
	var phase string
	if err = db.DB().QueryRow("SELECT phase FROM operations WHERE id=?", rec.ID).Scan(&phase); err != nil {
		t.Fatal(err)
	}
	if phase != string(contracts.OperationReserved) {
		t.Fatalf("cold operation changed %s", phase)
	}
	if _, err = os.Lstat(filepath.Join(path, ".git")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ticker initialized cold brain: %v", err)
	}
}

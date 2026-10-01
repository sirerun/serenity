package operation_test

import (
	"context"
	"errors"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/operation"
)

type blockingReconcileFence struct {
	firstEntered chan struct{}
	releaseFirst chan struct{}
	firstOnce    sync.Once
	calls        int32
}

func (f *blockingReconcileFence) EnterCommit(context.Context, string) (func(), error) {
	return nil, errors.New("unexpected EnterCommit call")
}

func (f *blockingReconcileFence) Fence(ctx context.Context, _ string) (func(), error) {
	call := atomic.AddInt32(&f.calls, 1)
	if call == 1 {
		f.firstOnce.Do(func() { close(f.firstEntered) })
		select {
		case <-f.releaseFirst:
			return func() {}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return func() {}, nil
}

type reconcileChecker func(context.Context, contracts.OperationRecord) (contracts.CanonicalVerdict, error)

func (f reconcileChecker) Check(ctx context.Context, record contracts.OperationRecord) (contracts.CanonicalVerdict, error) {
	return f(ctx, record)
}

func TestReconcileSkipsFinalizedCandidateAndContinuesBatch(t *testing.T) {
	s, account, brain := fixture(t)
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	clockMu := sync.RWMutex{}
	now := time.Unix(2_000_000_000, 0).UTC()
	clock := func() time.Time {
		clockMu.RLock()
		defer clockMu.RUnlock()
		return now
	}
	ledger := &operation.Ledger{Store: s, Clock: clock}
	request := contracts.ReserveRequest{
		AccountID: account, BrainID: brain, QuotaPeriod: "2026-10", Source: "gateway.remember", LeaseFor: time.Minute,
		Deltas: []contracts.ReserveDelta{{Metric: "writes", Units: 1, Limit: 3}},
	}
	rows := make([]contracts.OperationRecord, 3)
	var err error
	for i := range rows {
		rows[i], err = ledger.Reserve(context.Background(), request)
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	staleBeforeCheck, staleDuringCheck, landed := rows[0], rows[1], rows[2]
	if _, err := ledger.EnterCanonical(context.Background(), landed.ID); err != nil {
		t.Fatal(err)
	}
	clockMu.Lock()
	now = now.Add(2 * time.Minute)
	clockMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	fence := &blockingReconcileFence{firstEntered: make(chan struct{}), releaseFirst: make(chan struct{})}
	checkerEntered := make(chan struct{})
	releaseChecker := make(chan struct{})
	reconcileDone := make(chan struct {
		report contracts.ReconcileReport
		err    error
	}, 1)
	checkerCalls := make(chan string, 3)
	checker := reconcileChecker(func(ctx context.Context, record contracts.OperationRecord) (contracts.CanonicalVerdict, error) {
		checkerCalls <- record.ID
		switch record.ID {
		case staleDuringCheck.ID:
			close(checkerEntered)
			select {
			case <-releaseChecker:
				return contracts.CanonicalVerdict{Outcome: contracts.CanonicalUnknown, Ref: "fact_not_found"}, nil
			case <-ctx.Done():
				return contracts.CanonicalVerdict{}, ctx.Err()
			}
		case landed.ID:
			return contracts.CanonicalVerdict{Outcome: contracts.CanonicalLanded, Ref: "fact:sha256"}, nil
		default:
			return contracts.CanonicalVerdict{Outcome: contracts.CanonicalUnknown, Ref: "unexpected_candidate"}, nil
		}
	})
	go func() {
		report, err := ledger.ReconcilePending(ctx, checker, fence)
		reconcileDone <- struct {
			report contracts.ReconcileReport
			err    error
		}{report: report, err: err}
	}()
	select {
	case <-fence.firstEntered:
	case <-ctx.Done():
		t.Fatalf("reconciler did not reach the first fence: %v", ctx.Err())
	}

	finalized := make(chan error, 1)
	go func() {
		_, err := ledger.Finalize(ctx, staleBeforeCheck.ID, contracts.OperationReleased, contracts.Evidence{Kind: contracts.EvidenceNoCanonicalAttempt})
		finalized <- err
	}()
	select {
	case err := <-finalized:
		if err != nil {
			t.Fatalf("finalize no-entry candidate while fence waits: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("finalize blocked behind candidate query while fence waits: %v", ctx.Err())
	}
	close(fence.releaseFirst)
	select {
	case <-checkerEntered:
	case <-ctx.Done():
		t.Fatalf("reconciler did not proceed to the next reserved row: %v", ctx.Err())
	}
	finalizedDuringCheck := make(chan error, 1)
	go func() {
		_, err := ledger.Finalize(ctx, staleDuringCheck.ID, contracts.OperationReleased, contracts.Evidence{Kind: contracts.EvidenceNoCanonicalAttempt})
		finalizedDuringCheck <- err
	}()
	select {
	case err := <-finalizedDuringCheck:
		if err != nil {
			t.Fatalf("finalize no-entry candidate while checker runs: %v", err)
		}
	case <-ctx.Done():
		t.Fatalf("finalize blocked while checker runs: %v", ctx.Err())
	}
	close(releaseChecker)

	select {
	case result := <-reconcileDone:
		if result.err != nil {
			t.Fatalf("ReconcilePending: %v", result.err)
		}
		if result.report.Committed != 1 || result.report.Released != 0 || result.report.PendingReview != 0 {
			t.Fatalf("reconcile report=%+v, want only the still-reserved landed row committed", result.report)
		}
	case <-ctx.Done():
		t.Fatalf("reconcile did not finish the batch: %v", ctx.Err())
	}
	select {
	case got := <-checkerCalls:
		if got != staleDuringCheck.ID {
			t.Fatalf("first checker call was %q, want still-reserved row %q", got, staleDuringCheck.ID)
		}
	default:
		t.Fatal("checker was not called for the remaining eligible row")
	}
	select {
	case got := <-checkerCalls:
		if got != landed.ID {
			t.Fatalf("second checker call was %q, want landed row %q", got, landed.ID)
		}
	default:
		t.Fatal("checker did not continue to the remaining eligible row")
	}
	select {
	case got := <-checkerCalls:
		t.Fatalf("checker was called unexpectedly for %q", got)
	default:
	}
	var stalePhase, landedPhase string
	if err := s.DB().QueryRow(`SELECT phase FROM operations WHERE id=?`, staleBeforeCheck.ID).Scan(&stalePhase); err != nil {
		t.Fatal(err)
	}
	var midPhase string
	if err := s.DB().QueryRow(`SELECT phase FROM operations WHERE id=?`, staleDuringCheck.ID).Scan(&midPhase); err != nil {
		t.Fatal(err)
	}
	if err := s.DB().QueryRow(`SELECT phase FROM operations WHERE id=?`, landed.ID).Scan(&landedPhase); err != nil {
		t.Fatal(err)
	}
	if stalePhase != string(contracts.OperationReleased) || midPhase != string(contracts.OperationReleased) || landedPhase != string(contracts.OperationCommitted) {
		t.Fatalf("phases stale=%q mid=%q landed=%q, want released/released/committed", stalePhase, midPhase, landedPhase)
	}
}

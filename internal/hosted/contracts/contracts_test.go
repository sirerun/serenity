package contracts_test

import (
	"context"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

// These fixtures prove every interface in this package is actually
// implementable with the exact frozen signature — a plain type assertion
// alone would not catch a signature typo a dependent task's real
// implementation would trip on.

type fakeBillingReconciler struct{}

func (fakeBillingReconciler) ReconcileCustomer(context.Context, string) (contracts.ReconcileResult, error) {
	return contracts.ReconcileResult{}, nil
}

type fakeBillingCloser struct{}

func (fakeBillingCloser) CloseBillingAccount(context.Context, string) (contracts.CloseResult, error) {
	return contracts.CloseResult{}, nil
}

type fakeOperationLedger struct{}

func (fakeOperationLedger) Reserve(context.Context, contracts.ReserveRequest) (contracts.OperationRecord, error) {
	return contracts.OperationRecord{}, nil
}
func (fakeOperationLedger) EnterCanonical(context.Context, string) (contracts.OperationRecord, error) {
	return contracts.OperationRecord{}, nil
}
func (fakeOperationLedger) Finalize(context.Context, string, contracts.OperationPhase, contracts.Evidence) (contracts.OperationRecord, error) {
	return contracts.OperationRecord{}, nil
}
func (fakeOperationLedger) ReconcilePending(context.Context, contracts.CanonicalChecker, contracts.BrainFence) (contracts.ReconcileReport, error) {
	return contracts.ReconcileReport{}, nil
}
func (fakeOperationLedger) ResolvePendingReview(context.Context, string, contracts.OperationPhase, contracts.Evidence) (contracts.OperationRecord, error) {
	return contracts.OperationRecord{}, nil
}

type fakeDeletionJournal struct{}

func (fakeDeletionJournal) AppendDeletion(context.Context, contracts.DeletionEntry) (contracts.DeletionEntry, error) {
	return contracts.DeletionEntry{}, nil
}
func (fakeDeletionJournal) ReadThrough(context.Context, contracts.DeletionWatermark) (contracts.DeletionRead, error) {
	return contracts.DeletionRead{}, nil
}
func (fakeDeletionJournal) Seal(context.Context, int64) (contracts.DeletionWatermark, error) {
	return contracts.DeletionWatermark{}, nil
}

type fakeStagingGate struct{}

func (fakeStagingGate) ReserveStage(context.Context, contracts.StageRequest) (contracts.StageTicket, error) {
	return contracts.StageTicket{}, nil
}
func (fakeStagingGate) AdmitMeasured(context.Context, contracts.StageTicket, int64) error { return nil }
func (fakeStagingGate) Release(context.Context, contracts.StageTicket, contracts.StageOutcome) error {
	return nil
}

type fakeCanonicalChecker struct{}

func (fakeCanonicalChecker) Check(context.Context, contracts.OperationRecord) (contracts.CanonicalVerdict, error) {
	return contracts.CanonicalVerdict{}, nil
}

type fakeBrainFence struct{}

func (fakeBrainFence) EnterCommit(context.Context, string) (func(), error) { return func() {}, nil }
func (fakeBrainFence) Fence(context.Context, string) (func(), error)       { return func() {}, nil }

type fakeWriterFencer struct{}

func (fakeWriterFencer) Fence(context.Context, int64) (contracts.FenceReceipt, error) {
	return contracts.FenceReceipt{}, nil
}

type fakeJournalObjectStore struct{}

func (fakeJournalObjectStore) PutIfAbsent(context.Context, string, []byte) (bool, error) {
	return false, nil
}
func (fakeJournalObjectStore) Get(context.Context, string) ([]byte, bool, error) {
	return nil, false, nil
}
func (fakeJournalObjectStore) ListAfter(context.Context, string, string, int) ([]string, bool, error) {
	return nil, false, nil
}

type fakeTelemetry struct{}

func (fakeTelemetry) Emit(context.Context, contracts.TelemetryEvent) error { return nil }

var (
	_ contracts.BillingReconciler  = fakeBillingReconciler{}
	_ contracts.BillingCloser      = fakeBillingCloser{}
	_ contracts.OperationLedger    = fakeOperationLedger{}
	_ contracts.DeletionJournal    = fakeDeletionJournal{}
	_ contracts.StagingGate        = fakeStagingGate{}
	_ contracts.CanonicalChecker   = fakeCanonicalChecker{}
	_ contracts.BrainFence         = fakeBrainFence{}
	_ contracts.WriterFencer       = fakeWriterFencer{}
	_ contracts.JournalObjectStore = fakeJournalObjectStore{}
	_ contracts.Telemetry          = fakeTelemetry{}
)

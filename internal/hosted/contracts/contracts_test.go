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
func (fakeOperationLedger) Finalize(context.Context, string, contracts.OperationPhase, string) (contracts.OperationRecord, error) {
	return contracts.OperationRecord{}, nil
}
func (fakeOperationLedger) ReconcilePending(context.Context) (contracts.ReconcileReport, error) {
	return contracts.ReconcileReport{}, nil
}

type fakeDeletionJournal struct{}

func (fakeDeletionJournal) Append(context.Context, contracts.DeletionEntry) (contracts.DeletionEntry, error) {
	return contracts.DeletionEntry{}, nil
}
func (fakeDeletionJournal) ReadThrough(context.Context, contracts.DeletionWatermark) ([]contracts.DeletionEntry, contracts.DeletionWatermark, error) {
	return nil, contracts.DeletionWatermark{}, nil
}

type fakeAdmissionChecker struct{}

func (fakeAdmissionChecker) ReserveGrowth(context.Context, contracts.GrowthEnvelope) error {
	return nil
}

type fakeTelemetry struct{}

func (fakeTelemetry) Emit(context.Context, contracts.TelemetryEvent) error { return nil }

var (
	_ contracts.BillingReconciler = fakeBillingReconciler{}
	_ contracts.BillingCloser     = fakeBillingCloser{}
	_ contracts.OperationLedger   = fakeOperationLedger{}
	_ contracts.DeletionJournal   = fakeDeletionJournal{}
	_ contracts.AdmissionChecker  = fakeAdmissionChecker{}
	_ contracts.Telemetry         = fakeTelemetry{}
)

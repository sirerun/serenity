package service

import (
	"context"
	"log/slog"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

const operationSweepTimeout = 5 * time.Second

// reconcileOperations handles only expired reserved rows. Unknown evidence
// remains capacity-holding pending_review; no automatic operator resolution or
// absence verdict is introduced.
func (s *Service) reconcileOperations(ctx context.Context) (contracts.ReconcileReport, error) {
	sweepCtx, cancel := context.WithTimeout(ctx, operationSweepTimeout)
	defer cancel()
	report, err := s.operations.ReconcilePending(sweepCtx, s.operationReconciler, s.operationReconciler)
	if err == nil {
		err = sweepCtx.Err()
	}
	return report, err
}

func (s *Service) startOperationReconciler() {
	ctx, cancel := context.WithCancel(context.Background())
	s.operationWorkerCancel = cancel
	s.operationWorkerDone = make(chan struct{})
	go func() {
		defer close(s.operationWorkerDone)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		runOperationReconcileLoop(ctx, ticker.C, func(ctx context.Context) error {
			_, err := s.reconcileOperations(ctx)
			return err
		})
	}()
}

func runOperationReconcileLoop(ctx context.Context, ticks <-chan time.Time, run func(context.Context) error) {
	for {
		select {
		case <-ctx.Done():
			return
		case _, ok := <-ticks:
			if !ok || ctx.Err() != nil {
				return
			}
			if err := run(ctx); err != nil && ctx.Err() == nil {
				// Upstream error strings can contain source paths; keep logs fixed.
				slog.Warn("hosted operation reconciliation failed", "failure_class", "sweep")
			}
		}
	}
}

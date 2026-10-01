package service

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

const (
	billingReconcilePageSize  = 20
	billingCustomerTimeout    = 20 * time.Second
	billingPageInterval       = time.Minute
	billingSweepInterval      = 15 * time.Minute
	billingTransientRetryBase = time.Minute
	billingTransientRetryMax  = time.Hour
	billingAmbiguousRetryBase = 15 * time.Minute
	billingAmbiguousRetryMax  = 6 * time.Hour
	billingRetryCapacity      = 1024
)

type billingFailureClass string

const (
	billingFailureAmbiguous   billingFailureClass = "ambiguous_provider_state"
	billingFailureUnavailable billingFailureClass = "provider_unavailable"
	billingFailureTimeout     billingFailureClass = "timeout"
	billingFailureOther       billingFailureClass = "reconciliation_error"
)

type billingRetry struct {
	attempt int
	next    time.Time
	touched time.Time
	class   billingFailureClass
}

type billingRetries struct {
	byAccount map[string]billingRetry
}

func (r *billingRetries) ready(id string, now time.Time) bool {
	entry, ok := r.byAccount[id]
	if !ok || !entry.next.After(now) {
		return true
	}
	entry.touched = now
	r.byAccount[id] = entry
	return false
}

func (r *billingRetries) failed(id string, now time.Time, class billingFailureClass) {
	if r.byAccount == nil {
		r.byAccount = make(map[string]billingRetry)
	}
	entry := r.byAccount[id]
	if entry.class != class {
		entry = billingRetry{class: class}
	}
	entry.attempt++
	entry.touched = now
	delay := billingRetryDelay(class, entry.attempt)
	entry.next = now.Add(delay)
	if _, exists := r.byAccount[id]; !exists && len(r.byAccount) >= billingRetryCapacity {
		var oldestID string
		var oldest time.Time
		for key, candidate := range r.byAccount {
			if oldestID == "" || candidate.touched.Before(oldest) {
				oldestID, oldest = key, candidate.touched
			}
		}
		delete(r.byAccount, oldestID)
	}
	r.byAccount[id] = entry
}

func (r *billingRetries) succeeded(id string) { delete(r.byAccount, id) }

func billingRetryDelay(class billingFailureClass, attempt int) time.Duration {
	base, maximum := billingTransientRetryBase, billingTransientRetryMax
	if class == billingFailureAmbiguous {
		base, maximum = billingAmbiguousRetryBase, billingAmbiguousRetryMax
	}
	if attempt < 1 {
		attempt = 1
	}
	delay := base
	for n := 1; n < attempt && delay < maximum; n++ {
		if delay > maximum/2 {
			return maximum
		}
		delay *= 2
	}
	if delay > maximum {
		return maximum
	}
	return delay
}

func billingFailureFor(err error) billingFailureClass {
	switch {
	case errors.Is(err, contracts.ErrBillingProviderAmbiguous):
		return billingFailureAmbiguous
	case errors.Is(err, contracts.ErrBillingProviderUnavailable):
		return billingFailureUnavailable
	case errors.Is(err, context.DeadlineExceeded):
		return billingFailureTimeout
	default:
		return billingFailureOther
	}
}

func (s *Service) startBillingReconciler() {
	ctx, cancel := context.WithCancel(context.Background())
	s.billingWorkerCancel = cancel
	s.billingWorkerDone = make(chan struct{})
	go func() {
		defer close(s.billingWorkerDone)
		s.billingReconcileLoop(ctx)
	}()
}

func (s *Service) billingReconcileLoop(ctx context.Context) {
	retries := billingRetries{byAccount: make(map[string]billingRetry)}
	after := ""
	pageFailures := 0
	for ctx.Err() == nil {
		complete, err := s.runBillingSweep(ctx, &retries, &after)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			pageFailures++
			delay := billingRetryDelay(billingFailureOther, pageFailures)
			slog.Error("hosted billing reconciliation page failed; retrying", "failure_class", "page_query", "retry_after", delay)
			if !waitContext(ctx, delay) {
				return
			}
			continue
		}
		pageFailures = 0
		if complete {
			after = ""
			if !waitContext(ctx, billingSweepInterval) {
				return
			}
		}
	}
}

func (s *Service) runBillingSweep(ctx context.Context, retries *billingRetries, after *string) (bool, error) {
	for ctx.Err() == nil {
		complete, err := runBillingPage(ctx, retries, after, s.billingAccountPage, s.reconcileBillingAccount)
		if err != nil || complete {
			return complete, err
		}
		if !waitContext(ctx, billingPageInterval) {
			return false, ctx.Err()
		}
	}
	return false, ctx.Err()
}

func runBillingPage(
	ctx context.Context,
	retries *billingRetries,
	after *string,
	readPage func(context.Context, string) ([]string, error),
	reconcile func(context.Context, string) error,
) (bool, error) {
	ids, err := readPage(ctx, *after)
	if err != nil {
		return false, err
	}
	if len(ids) == 0 {
		return true, nil
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		if !retries.ready(id, time.Now()) {
			continue
		}
		if err = reconcile(ctx, id); err != nil {
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			class := billingFailureFor(err)
			retries.failed(id, time.Now(), class)
			slog.Warn("hosted billing customer reconciliation failed; retry scheduled", "failure_class", class)
		} else {
			retries.succeeded(id)
		}
	}
	// Advance only after every row in the page has been classified. A page
	// query error leaves this cursor unchanged so later IDs cannot be starved.
	*after = ids[len(ids)-1]
	return len(ids) < billingReconcilePageSize, nil
}

// billingAccountPage closes its cursor before returning IDs, so a single-connection
// SQLite pool is available for reconciliation and provider work.
func (s *Service) billingAccountPage(ctx context.Context, after string) ([]string, error) {
	rows, err := s.Store.DB().QueryContext(ctx, `SELECT id FROM accounts
		WHERE id>? AND status IN ('active','restore_pending')
		AND stripe_customer_id IS NOT NULL AND stripe_customer_id<>''
		ORDER BY id LIMIT ?`, after, billingReconcilePageSize)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, billingReconcilePageSize)
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *Service) reconcileBillingAccount(parent context.Context, accountID string) error {
	status, err := s.billingAccountStatus(parent, accountID)
	if err != nil {
		return err
	}
	if status != "active" && status != "restore_pending" {
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, billingCustomerTimeout)
	defer cancel()
	_, reconcileErr := s.billingReconciler.ReconcileCustomer(ctx, accountID)
	if errors.Is(reconcileErr, contracts.ErrBillingAccountFrozen) {
		current, readErr := s.billingAccountStatus(ctx, accountID)
		if readErr == nil && current == "restore_pending" {
			return nil // Frozen bookkeeping is expected; restore remains restricted.
		}
	}
	if reconcileErr != nil {
		return reconcileErr
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	current, err := s.billingAccountStatus(ctx, accountID)
	if err != nil {
		return err
	}
	if current != "active" {
		// ReconcileCustomer can have read "active" before a concurrent freeze.
		// Repair only the derived plan projection; never change lifecycle state.
		_, err = s.Store.DB().ExecContext(ctx, `UPDATE accounts SET plan_id='free',plan_version=1 WHERE id=? AND status<>'active'`, accountID)
		return err
	}
	return nil
}

func (s *Service) billingAccountStatus(ctx context.Context, accountID string) (string, error) {
	var status string
	err := s.Store.DB().QueryRowContext(ctx, `SELECT status FROM accounts WHERE id=?`, accountID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return status, err
}

func waitContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

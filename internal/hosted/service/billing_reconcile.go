package service

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

const (
	billingReconcilePageSize = 20
	billingCustomerTimeout   = 20 * time.Second
	billingPageInterval      = time.Minute
	billingSweepInterval     = 15 * time.Minute
	billingRetryMax          = 15 * time.Minute
	billingRetryCapacity     = 1024
)

type billingRetry struct {
	attempt int
	next    time.Time
	touched time.Time
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

func (r *billingRetries) failed(id string, now time.Time) {
	if r.byAccount == nil {
		r.byAccount = make(map[string]billingRetry)
	}
	entry := r.byAccount[id]
	entry.attempt++
	entry.touched = now
	delay := time.Minute
	for n := 1; n < entry.attempt && delay < billingRetryMax; n++ {
		delay *= 2
	}
	if delay > billingRetryMax {
		delay = billingRetryMax
	}
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
	for ctx.Err() == nil {
		sweepStarted := time.Now()
		_ = s.runBillingSweep(ctx, &retries)
		wait := billingSweepInterval - time.Since(sweepStarted)
		if wait > 0 && !waitContext(ctx, wait) {
			return
		}
	}
}

func (s *Service) runBillingSweep(ctx context.Context, retries *billingRetries) error {
	after := ""
	for ctx.Err() == nil {
		ids, err := s.billingAccountPage(ctx, after)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		for _, id := range ids {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			after = id
			if !retries.ready(id, time.Now()) {
				continue
			}
			if err = s.reconcileBillingAccount(ctx, id); err != nil {
				retries.failed(id, time.Now())
			} else {
				retries.succeeded(id)
			}
		}
		if len(ids) < billingReconcilePageSize {
			return nil
		}
		if !waitContext(ctx, billingPageInterval) {
			return ctx.Err()
		}
	}
	return ctx.Err()
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

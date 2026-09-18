// Package meter reserves account-wide allowances before memory operations.
package meter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/sirerun/serenity/internal/hosted/plans"
	"github.com/sirerun/serenity/internal/hosted/store"
)

var ErrInProgress = errors.New("operation is already in progress")

type LimitError struct {
	Metric  string
	ResetAt time.Time
}

func (e *LimitError) Error() string { return "limit_exceeded: " + e.Metric }

type Meter struct {
	Store *store.Store
	Clock func() time.Time
}
type Entitlement struct {
	Plan    plans.Plan
	Window  string
	ResetAt time.Time
}

func (m *Meter) now() time.Time {
	if m.Clock != nil {
		return m.Clock().UTC()
	}
	return time.Now().UTC()
}
func (m *Meter) Entitlement(ctx context.Context, accountID string) (Entitlement, error) {
	now := m.now()
	out := Entitlement{Plan: plans.Get("free"), Window: now.Format("2006-01"), ResetAt: time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC)}
	var status, plan string
	if err := m.Store.DB().QueryRowContext(ctx, `SELECT status,plan_id FROM accounts WHERE id=?`, accountID).Scan(&status, &plan); err != nil {
		return out, err
	}
	if status != "active" {
		return out, store.ErrNotFound
	}
	// Paid access requires a reconciled live subscription, never a browser return.
	var start, end, subscriptionStatus string
	err := m.Store.DB().QueryRowContext(ctx, `SELECT current_period_start,current_period_end,status FROM subscriptions WHERE account_id=? ORDER BY current_period_end DESC LIMIT 1`, accountID).Scan(&start, &end, &subscriptionStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	until, err := time.Parse(time.RFC3339Nano, end)
	if err != nil {
		return out, err
	}
	if (subscriptionStatus == "active" || subscriptionStatus == "trialing") && until.After(now) {
		out.Plan = plans.Get(plan)
		out.Window = start
		out.ResetAt = until
	}
	return out, nil
}

type Reservation struct {
	ID     string
	Replay bool
}

func (m *Meter) Reserve(ctx context.Context, accountID, metric string, amount, limit int64, window string, reset time.Time, operationKey string) (r Reservation, err error) {
	if amount < 0 || limit < 0 {
		return r, errors.New("invalid reservation amount")
	}
	now := m.now()
	err = m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `UPDATE reservations SET status='released' WHERE status='open' AND lease_expires_at<=?`, store.Stamp(now))
		if e != nil {
			return e
		}
		if operationKey != "" {
			var status string
			e = tx.QueryRowContext(ctx, `SELECT id,status FROM reservations WHERE account_id=? AND metric=? AND operation_key=? AND status!='released' ORDER BY created_at LIMIT 1`, accountID, metric, operationKey).Scan(&r.ID, &status)
			if e == nil {
				if status == "committed" {
					r.Replay = true
					return nil
				}
				return ErrInProgress
			}
			if !errors.Is(e, sql.ErrNoRows) {
				return e
			}
		}
		var used, open int64
		e = tx.QueryRowContext(ctx, `SELECT COALESCE((SELECT committed FROM usage_windows WHERE account_id=? AND window_key=? AND metric=?),0)`, accountID, window, metric).Scan(&used)
		if e != nil {
			return e
		}
		e = tx.QueryRowContext(ctx, `SELECT COALESCE(sum(amount),0) FROM reservations WHERE account_id=? AND window_key=? AND metric=? AND status='open'`, accountID, window, metric).Scan(&open)
		if e != nil {
			return e
		}
		if amount > limit || used > limit-amount || open > limit-amount-used {
			return &LimitError{metric, reset}
		}
		r.ID = store.ID()
		var key any
		if operationKey != "" {
			key = operationKey
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO reservations(id,account_id,window_key,metric,amount,operation_key,status,lease_expires_at,created_at) VALUES(?,?,?,?,?,?,'open',?,?)`, r.ID, accountID, window, metric, amount, key, store.Stamp(now.Add(5*time.Minute)), store.Stamp(now))
		return e
	})
	return
}
func (m *Meter) Finish(ctx context.Context, r Reservation, success bool) error {
	if r.Replay {
		return nil
	}
	return m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var account, window, metric, status string
		var amount int64
		e := tx.QueryRowContext(ctx, `SELECT account_id,window_key,metric,amount,status FROM reservations WHERE id=?`, r.ID).Scan(&account, &window, &metric, &amount, &status)
		if e != nil {
			return e
		}
		if status != "open" {
			if status == "committed" && success {
				return nil
			}
			return fmt.Errorf("reservation is %s", status)
		}
		target := "released"
		if success {
			target = "committed"
			_, e = tx.ExecContext(ctx, `INSERT INTO usage_windows(account_id,window_key,metric,committed) VALUES(?,?,?,?) ON CONFLICT(account_id,window_key,metric) DO UPDATE SET committed=committed+excluded.committed`, account, window, metric, amount)
			if e != nil {
				return e
			}
		}
		_, e = tx.ExecContext(ctx, `UPDATE reservations SET status=? WHERE id=?`, target, r.ID)
		return e
	})
}

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
	// Bucket is BucketAccount or BucketPartner; Subject names the meter
	// subject: the account ID, or partner:<id>:<account_id>.
	Bucket  string
	Subject string
}

const (
	BucketAccount = "account"
	BucketPartner = "partner"
	// PartnerProPlan is the plan whose limits apply to partner-bound traffic
	// under a live partner "pro" entitlement (ADR 023).
	PartnerProPlan = "builder"
)

// PartnerWindow is the quota period of a partner bucket: the UTC calendar
// month, namespaced by partner so it can never collide with an account
// window. The account ID stays the row owner in usage_windows, reservations
// and operations, so the partner subject partner:<id>:<account_id> is the
// pair (account_id, PartnerWindow).
func PartnerWindow(partnerID string, now time.Time) string {
	return "partner:" + partnerID + ":" + now.UTC().Format("2006-01")
}

// PartnerEntitlement returns the partner bucket for accountID when the
// partner has a live "pro" entitlement for it, an active link, and is itself
// active. ok is false otherwise, and partner traffic then uses the account.
func (m *Meter) PartnerEntitlement(ctx context.Context, accountID, partnerID string) (out Entitlement, ok bool, err error) {
	if partnerID == "" {
		return out, false, nil
	}
	now := m.now()
	var tier, expires string
	err = m.Store.DB().QueryRowContext(ctx, `SELECT e.tier,COALESCE(e.expires_at,'') FROM partner_entitlements e JOIN partners p ON p.id=e.partner_id JOIN partner_links l ON l.partner_id=e.partner_id AND l.account_id=e.account_id WHERE e.account_id=? AND e.partner_id=? AND p.status='active' AND l.status='active'`, accountID, partnerID).Scan(&tier, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return out, false, nil
	}
	if err != nil {
		return out, false, err
	}
	if tier != "pro" {
		return out, false, nil
	}
	if expires != "" {
		until, e := time.Parse(time.RFC3339Nano, expires)
		if e != nil {
			return out, false, e
		}
		if !until.After(now) {
			return out, false, nil
		}
	}
	return Entitlement{
		Plan:    plans.Get(PartnerProPlan),
		Window:  PartnerWindow(partnerID, now),
		ResetAt: time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC),
		Bucket:  BucketPartner,
		Subject: "partner:" + partnerID + ":" + accountID,
	}, true, nil
}

// Subject selects the meter subject for one call. subject is the bucket the
// call's operations are charged to; account is always the account's own plan,
// which inventory checks need because inventory is physically account-wide.
func (m *Meter) Subject(ctx context.Context, accountID, partnerID string) (subject, account Entitlement, err error) {
	account, err = m.Entitlement(ctx, accountID)
	if err != nil {
		return subject, account, err
	}
	partner, ok, err := m.PartnerEntitlement(ctx, accountID, partnerID)
	if err != nil {
		return subject, account, err
	}
	if ok {
		return partner, account, nil
	}
	return account, account, nil
}

func (m *Meter) now() time.Time {
	if m.Clock != nil {
		return m.Clock().UTC()
	}
	return time.Now().UTC()
}
func (m *Meter) Entitlement(ctx context.Context, accountID string) (Entitlement, error) {
	now := m.now()
	out := Entitlement{Plan: plans.Get("free"), Window: now.Format("2006-01"), ResetAt: time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, time.UTC), Bucket: BucketAccount, Subject: accountID}
	var status, plan string
	if err := m.Store.DB().QueryRowContext(ctx, `SELECT status,plan_id FROM accounts WHERE id=?`, accountID).Scan(&status, &plan); err != nil {
		return out, err
	}
	if status != "active" {
		return out, store.ErrNotFound
	}
	// Paid access requires a reconciled live subscription, never a browser return.
	var start, end, subscriptionStatus, grace string
	err := m.Store.DB().QueryRowContext(ctx, `SELECT current_period_start,current_period_end,status,plan_id,COALESCE(grace_until,'') FROM subscriptions WHERE account_id=? AND (status IN ('active','trialing') OR (status='past_due' AND grace_until>?)) ORDER BY current_period_end DESC LIMIT 1`, accountID, store.Stamp(now)).Scan(&start, &end, &subscriptionStatus, &plan, &grace)
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
	if subscriptionStatus == "past_due" {
		deadline, e := time.Parse(time.RFC3339Nano, grace)
		if e != nil {
			return out, e
		}
		if deadline.After(now) {
			out.Plan = plans.Get(plan)
			out.Window = start
			out.ResetAt = deadline
		}
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
		if _, e = tx.ExecContext(ctx, `DELETE FROM reservations WHERE status='released' AND created_at<?`, store.Stamp(now.Add(-24*time.Hour))); e != nil {
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

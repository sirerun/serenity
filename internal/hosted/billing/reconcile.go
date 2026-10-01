// Package billing reconciles hosted entitlements from Stripe, never return URLs.
package billing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/store"
	"github.com/sirerun/serenity/internal/hosted/testhooks"
	"net/url"
	"strings"
	"time"
)

func (s *Service) readPriorSubscription(ctx context.Context, tx *sql.Tx, subscriptionID, accountID string) (priorSubscription, error) {
	var prior priorSubscription
	var err error
	if s.priorSubscriptionReader != nil {
		prior, err = s.priorSubscriptionReader(ctx, tx, subscriptionID, accountID)
	} else {
		err = tx.QueryRowContext(ctx, `SELECT status,COALESCE(grace_until,''),COALESCE(grace_invoice_id,''),COALESCE(current_period_start,''),COALESCE(current_period_end,'') FROM subscriptions WHERE id=? AND account_id=?`, subscriptionID, accountID).Scan(&prior.status, &prior.grace, &prior.graceInvoice, &prior.periodStart, &prior.periodEnd)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return priorSubscription{}, nil
	}
	if err != nil {
		return priorSubscription{}, &persistedBillingReadError{operation: "read prior subscription state", err: err}
	}
	return prior, nil
}

func (s *Service) reconcileOldCheckoutAttempt(ctx context.Context, accountID string, hasSubscription bool) error {
	var attemptID, sessionID, created, requestBody, customer string
	var requestVersion int
	err := s.Store.DB().QueryRowContext(ctx, `SELECT c.id,COALESCE(c.session_id,''),c.created_at,c.request_version,COALESCE(c.request_body,''),COALESCE(a.stripe_customer_id,'') FROM checkout_attempts c JOIN accounts a ON a.id=c.account_id WHERE c.account_id=?`, accountID).Scan(&attemptID, &sessionID, &created, &requestVersion, &requestBody, &customer)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if sessionID == "" {
		if requestVersion != 1 || requestBody == "" || customer == "" {
			return fmt.Errorf("%w: legacy checkout creation outcome unresolved", contracts.ErrBillingProviderAmbiguous)
		}
		recovered, ok, e := s.discoverCheckoutAttempt(ctx, accountID, customer, attemptID)
		if e != nil {
			return e
		}
		if !ok {
			return fmt.Errorf("%w: checkout session not yet discoverable", contracts.ErrBillingProviderAmbiguous)
		}
		if e = s.saveRecoveredCheckoutID(ctx, accountID, attemptID, recovered.ID); e != nil {
			return e
		}
		sessionID = recovered.ID
	}
	at, err := time.Parse(time.RFC3339Nano, created)
	if err != nil || time.Since(at) <= 23*time.Hour {
		return nil
	}
	state := "missing_session"
	if sessionID != "" {
		if !strings.HasPrefix(sessionID, "cs_") || strings.ContainsAny(sessionID, "/?#") {
			return fmt.Errorf("%w: invalid checkout session reference", contracts.ErrBillingProviderAmbiguous)
		}
		var session struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}
		if err = s.providerRequest(ctx, "GET", "/checkout/sessions/"+url.PathEscape(sessionID), nil, "", &session); err != nil {
			return err
		}
		if session.ID != sessionID {
			return fmt.Errorf("%w: checkout session identity mismatch", contracts.ErrBillingProviderAmbiguous)
		}
		switch session.Status {
		case "open":
			session.ID, session.Status = "", ""
			if err = s.providerRequest(ctx, "POST", "/checkout/sessions/"+url.PathEscape(sessionID)+"/expire", nil, "serenity-expire-"+sessionID, &session); err != nil {
				return err
			}
			if session.ID != sessionID || session.Status != "expired" {
				return fmt.Errorf("%w: checkout expiration not confirmed", contracts.ErrBillingProviderAmbiguous)
			}
			state = "expired_open_session"
		case "complete":
			if !hasSubscription {
				return fmt.Errorf("%w: completed checkout has no subscription", contracts.ErrBillingProviderAmbiguous)
			}
			state = "completed_session"
		case "expired":
			state = "expired_session"
		default:
			return fmt.Errorf("%w: checkout session unresolved", contracts.ErrBillingProviderAmbiguous)
		}
	}
	return s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM checkout_attempts WHERE account_id=? AND id=?`, accountID, attemptID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO audit_log(account_id,actor,action,created_at,detail) VALUES(?,?,?,?,?)`, accountID, "billing-reconciler", "checkout_attempt_reconciled", store.Stamp(time.Now()), state)
		return err
	})
}

func (s *Service) planForPrice(price string) string {
	switch price {
	case s.Config.BuilderPrice:
		if price != "" {
			return "builder"
		}
	case s.Config.ScalePrice:
		if price != "" {
			return "scale"
		}
	}
	return ""
}

func subscriptionTerminal(status string) bool {
	return status == "canceled" || status == "incomplete_expired"
}

// effectiveSubscriptionAccess mirrors the hosted meter's billing eligibility:
// active/trialing subscriptions need a live period, while past-due access is
// limited to the exact persisted grace deadline.

func effectiveSubscriptionAccess(status string, periodEnd time.Time, graceUntil string, now time.Time) (bool, error) {
	switch status {
	case "active", "trialing":
		return periodEnd.After(now), nil
	case "past_due":
		if graceUntil == "" {
			return false, nil
		}
		deadline, err := time.Parse(time.RFC3339Nano, graceUntil)
		if err != nil {
			return false, fmt.Errorf("parse reconciled grace deadline: %w", err)
		}
		return deadline.After(now), nil
	default:
		return false, nil
	}
}

// graceDeadline derives a past-due deadline from the first failure event for
// the latest invoice. Reconciliation and webhook delivery use the same
// persisted provider timestamp, so event order and local processing delays
// cannot shift access. Recomputing also repairs deadlines previously stored
// by versions that used delivery time or period start.

func graceDeadline(oldStatus, oldGrace, oldInvoice string, periodChanged bool, newStatus, newInvoice string, anchor time.Time) (string, string) {
	if newStatus != "past_due" {
		return "", ""
	}
	if oldStatus == "past_due" && oldGrace != "" && !periodChanged && oldInvoice != "" && oldInvoice != newInvoice {
		return oldGrace, oldInvoice
	}
	deadline := store.Stamp(anchor.Add(72 * time.Hour))
	if oldStatus == "past_due" && oldGrace != "" && !periodChanged && oldInvoice == "" {
		// Legacy rows have no invoice identity. Correct a deadline that would
		// overgrant, but do not extend it based on an invoice we cannot prove
		// established the existing grace window.
		if prior, err := time.Parse(time.RFC3339Nano, oldGrace); err == nil && prior.Before(anchor.Add(72*time.Hour)) {
			return oldGrace, ""
		}
	}
	return deadline, newInvoice
}

// recordWindowClosed preserves the closing accounting window in the audit
// log before a renewal's fresh row overwrites current_period_start/end and
// grace_until in place. The subscriptions table holds only the live window;
// this is the retained history of the one a renewal replaces.

func recordWindowClosed(ctx context.Context, tx *sql.Tx, accountID, subscriptionID, status, periodStart, periodEnd, grace string) error {
	if grace == "" {
		grace = "none"
	}
	detail := fmt.Sprintf("subscription=%s status=%s period_start=%s period_end=%s grace_until=%s", subscriptionID, status, periodStart, periodEnd, grace)
	_, err := tx.ExecContext(ctx, `INSERT INTO audit_log(account_id,actor,action,created_at,detail) VALUES(?,?,?,?,?)`, accountID, "billing-reconciler", "subscription_window_closed", store.Stamp(time.Now()), detail)
	return err
}

// ReconcileCustomer fetches provider truth using only the customer recorded on
// the account, then replaces the local subscription projection atomically.
// Frozen accounts are reconciled for bookkeeping but never receive access.

func (s *Service) ReconcileCustomer(ctx context.Context, accountID string) (contracts.ReconcileResult, error) {
	release, err := s.locks.acquire(ctx, "account:"+accountID)
	if err != nil {
		return contracts.ReconcileResult{}, err
	}
	defer release()
	var result contracts.ReconcileResult
	var customer, status string
	if err := s.Store.DB().QueryRowContext(ctx, `SELECT COALESCE(stripe_customer_id,''),status FROM accounts WHERE id=?`, accountID).Scan(&customer, &status); err != nil {
		return result, err
	}
	if status != "active" && status != "restore_pending" && status != "deleting" {
		return result, contracts.ErrBillingAccountFrozen
	}
	if customer == "" {
		if status != "active" {
			return result, contracts.ErrBillingAccountFrozen
		}
		result.Source = "local_no_customer"
		return result, nil
	}
	list, err := s.listSubscriptions(ctx, customer)
	if err != nil {
		return result, err
	}
	var chosen *subscription
	for i := range list.Data {
		sub := &list.Data[i]
		item := sub.Items.Data[0]
		if s.planForPrice(item.Price.ID) == "" {
			return result, fmt.Errorf("%w: unknown provider price", contracts.ErrBillingProviderAmbiguous)
		}
		if !subscriptionTerminal(sub.Status) && sub.Status != "incomplete" {
			if chosen != nil {
				return result, fmt.Errorf("%w: duplicate nonterminal subscriptions", contracts.ErrBillingProviderAmbiguous)
			}
			chosen = sub
		}
	}
	if err = s.reconcileOldCheckoutAttempt(ctx, accountID, chosen != nil); err != nil {
		return result, err
	}
	testhooks.At(testhooks.PhaseReconcileFenced)
	type failureEvidence struct {
		invoice string
		at      time.Time
	}
	failures := make(map[string]failureEvidence, len(list.Data))
	for _, sub := range list.Data {
		if sub.Status != "past_due" {
			continue
		}
		anchor, e := s.failureEvidence(ctx, accountID, sub)
		if e != nil {
			return result, e
		}
		invoice, e := invoiceID(sub.LatestInvoice)
		if e != nil {
			return result, e
		}
		failures[sub.ID] = failureEvidence{invoice: invoice, at: anchor}
	}

	now := time.Now().UTC()
	var chosenGrace string
	var chosenEligible bool
	err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		rows, e := tx.QueryContext(ctx, `SELECT id FROM subscriptions WHERE account_id=?`, accountID)
		if e != nil {
			return e
		}
		var localIDs []string
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				_ = rows.Close()
				return e
			}
			localIDs = append(localIDs, id)
		}
		if e = errors.Join(rows.Err(), rows.Close()); e != nil {
			return e
		}
		seen := make(map[string]bool, len(list.Data))
		for _, sub := range list.Data {
			item := sub.Items.Data[0]
			plan := s.planForPrice(item.Price.ID)
			seen[sub.ID] = true
			old, readErr := s.readPriorSubscription(ctx, tx, sub.ID, accountID)
			if readErr != nil {
				return readErr
			}
			periodStart := time.Unix(item.CurrentPeriodStart, 0).UTC()
			anchor := periodStart
			invoice := ""
			if sub.Status == "past_due" {
				failure := failures[sub.ID]
				anchor, invoice = failure.at, failure.invoice
			}
			newPeriodStart := store.Stamp(periodStart)
			newPeriodEnd := store.Stamp(time.Unix(item.CurrentPeriodEnd, 0))
			periodChanged := old.periodStart != "" && old.periodStart != newPeriodStart
			if periodChanged {
				if e = recordWindowClosed(ctx, tx, accountID, sub.ID, old.status, old.periodStart, old.periodEnd, old.grace); e != nil {
					return e
				}
			}
			grace, graceInvoice := graceDeadline(old.status, old.grace, old.graceInvoice, periodChanged, sub.Status, invoice, anchor)
			if chosen != nil && sub.ID == chosen.ID {
				chosenGrace = grace
				chosenEligible, e = effectiveSubscriptionAccess(sub.Status, time.Unix(item.CurrentPeriodEnd, 0).UTC(), grace, now)
				if e != nil {
					return e
				}
			}
			_, e = tx.ExecContext(ctx, `INSERT INTO subscriptions(id,account_id,price_id,plan_id,status,current_period_start,current_period_end,cancel_at_period_end,grace_until,grace_invoice_id) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET price_id=excluded.price_id,plan_id=excluded.plan_id,status=excluded.status,current_period_start=excluded.current_period_start,current_period_end=excluded.current_period_end,cancel_at_period_end=excluded.cancel_at_period_end,grace_until=excluded.grace_until,grace_invoice_id=excluded.grace_invoice_id`, sub.ID, accountID, item.Price.ID, plan, sub.Status, newPeriodStart, newPeriodEnd, sub.CancelAtPeriodEnd, nullableString(grace), nullableString(graceInvoice))
			if e != nil {
				return e
			}
		}
		for _, id := range localIDs {
			if !seen[id] {
				if _, e = tx.ExecContext(ctx, `UPDATE subscriptions SET status='canceled',grace_until=NULL WHERE id=? AND account_id=?`, id, accountID); e != nil {
					return e
				}
			}
		}
		if chosen == nil || status != "active" || !chosenEligible {
			_, e = tx.ExecContext(ctx, `UPDATE accounts SET plan_id='free',plan_version=1 WHERE id=?`, accountID)
		} else {
			_, e = tx.ExecContext(ctx, `UPDATE accounts SET plan_id=?,plan_version=1 WHERE id=?`, s.planForPrice(chosen.Items.Data[0].Price.ID), accountID)
		}
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO audit_log(account_id,actor,action,created_at,detail) VALUES(?,?,?,?,?)`, accountID, "billing-reconciler", "billing_reconciled", store.Stamp(now), fmt.Sprintf("subscriptions=%d", len(list.Data)))
		return e
	})
	if err != nil {
		return result, err
	}
	if chosen == nil || status != "active" {
		if status != "active" {
			return result, contracts.ErrBillingAccountFrozen
		}
		result.Source = "stripe_customer"
		return result, nil
	}
	item := chosen.Items.Data[0]
	result.Eligible = chosenEligible
	result.PlanID = s.planForPrice(item.Price.ID)
	result.CurrentWindowStart = time.Unix(item.CurrentPeriodStart, 0).UTC()
	result.CurrentWindowEnd = time.Unix(item.CurrentPeriodEnd, 0).UTC()
	result.Source = "stripe_subscription"
	if chosen.Status == "past_due" {
		if chosenGrace != "" {
			result.GraceUntil, err = time.Parse(time.RFC3339Nano, chosenGrace)
			if err != nil {
				return contracts.ReconcileResult{}, fmt.Errorf("parse reconciled grace deadline: %w", err)
			}
		}
	}
	return result, nil
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// Package billing reconciles hosted entitlements from Stripe, never return URLs.
package billing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"net/url"
	"strings"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

func (s *Service) expireCheckout(ctx context.Context, sessionID string) error {
	if sessionID == "" || !strings.HasPrefix(sessionID, "cs_") || strings.ContainsAny(sessionID, "/?#") {
		return errors.New("invalid checkout session reference")
	}
	var session struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := s.providerRequest(ctx, "GET", "/checkout/sessions/"+url.PathEscape(sessionID), nil, "", &session); err != nil {
		return err
	}
	if session.ID != sessionID {
		return fmt.Errorf("%w: checkout session identity mismatch", contracts.ErrBillingProviderAmbiguous)
	}
	switch session.Status {
	case "expired", "complete":
		return nil
	case "open":
		session.ID, session.Status = "", ""
		if err := s.providerRequest(ctx, "POST", "/checkout/sessions/"+url.PathEscape(sessionID)+"/expire", nil, "serenity-expire-"+sessionID, &session); err != nil {
			return err
		}
		if session.ID != sessionID || session.Status != "expired" {
			return fmt.Errorf("%w: checkout expiration not confirmed", contracts.ErrBillingProviderAmbiguous)
		}
		return nil
	default:
		return fmt.Errorf("%w: checkout session %s unresolved", contracts.ErrBillingProviderAmbiguous, sessionID)
	}
}

// closeBilling is shared by deletion lifecycle callers and the legacy
// CancelAccount adapter. It certifies provider closure before returning.

func (s *Service) closeBilling(ctx context.Context, account string, requireDeleting bool) (contracts.CloseResult, error) {
	release, err := s.locks.acquire(ctx, "account:"+account)
	if err != nil {
		return contracts.CloseResult{Status: contracts.CloseStatusPending, PendingReason: "account operation lock unavailable"}, err
	}
	defer release()
	var customer, status string
	if err := s.Store.DB().QueryRowContext(ctx, `SELECT COALESCE(stripe_customer_id,''),status FROM accounts WHERE id=?`, account).Scan(&customer, &status); err != nil {
		return contracts.CloseResult{Status: contracts.CloseStatusPending, PendingReason: "account lookup failed"}, err
	}
	if requireDeleting && status != "deleting" {
		return contracts.CloseResult{Status: contracts.CloseStatusPending, PendingReason: "account is not deleting"}, contracts.ErrBillingAccountFrozen
	}
	if customer == "" {
		err := s.Store.Transaction(ctx, func(tx *sql.Tx) error {
			if _, e := tx.ExecContext(ctx, `UPDATE subscriptions SET status='canceled',grace_until=NULL WHERE account_id=? AND status NOT IN ('canceled','incomplete_expired')`, account); e != nil {
				return e
			}
			if _, e := tx.ExecContext(ctx, `DELETE FROM checkout_attempts WHERE account_id=?`, account); e != nil {
				return e
			}
			_, e := tx.ExecContext(ctx, `UPDATE accounts SET plan_id='free',plan_version=1 WHERE id=?`, account)
			return e
		})
		if err != nil {
			return contracts.CloseResult{Status: contracts.CloseStatusPending, PendingReason: "local checkout cleanup failed"}, err
		}
		return contracts.CloseResult{Status: contracts.CloseStatusClosed}, nil
	}
	list, err := s.listSubscriptions(ctx, customer)
	if err != nil {
		return contracts.CloseResult{Status: contracts.CloseStatusPending, PendingReason: "provider subscription truth unavailable"}, err
	}
	type pendingCheckout struct {
		attemptID, sessionID string
		requestVersion       int
		requestBody          string
	}
	var attempts []pendingCheckout
	rows, err := s.Store.DB().QueryContext(ctx, `SELECT id,COALESCE(session_id,''),request_version,COALESCE(request_body,'') FROM checkout_attempts WHERE account_id=?`, account)
	if err != nil {
		return contracts.CloseResult{Status: contracts.CloseStatusPending, PendingReason: "checkout lookup failed"}, err
	}
	for rows.Next() {
		var pending pendingCheckout
		if err = rows.Scan(&pending.attemptID, &pending.sessionID, &pending.requestVersion, &pending.requestBody); err != nil {
			_ = rows.Close()
			return contracts.CloseResult{Status: contracts.CloseStatusPending, PendingReason: "checkout lookup failed"}, err
		}
		attempts = append(attempts, pending)
	}
	if err = errors.Join(rows.Err(), rows.Close()); err != nil {
		return contracts.CloseResult{Status: contracts.CloseStatusPending, PendingReason: "checkout lookup failed"}, err
	}
	for index := range attempts {
		if attempts[index].sessionID != "" {
			continue
		}
		pending := attempts[index]
		if pending.requestVersion != 1 || pending.requestBody == "" {
			return contracts.CloseResult{Status: contracts.CloseStatusPending, PendingReason: "legacy checkout creation outcome unresolved"}, contracts.ErrBillingProviderAmbiguous
		}
		recovered, found, e := s.discoverCheckoutAttempt(ctx, account, customer, pending.attemptID)
		if e != nil {
			return contracts.CloseResult{Status: contracts.CloseStatusPending, PendingReason: "checkout discovery failed"}, e
		}
		if !found {
			return contracts.CloseResult{Status: contracts.CloseStatusPending, PendingReason: "checkout creation outcome unresolved"}, contracts.ErrBillingProviderAmbiguous
		}
		if e = s.saveRecoveredCheckoutID(ctx, account, pending.attemptID, recovered.ID); e != nil {
			return contracts.CloseResult{Status: contracts.CloseStatusPending, PendingReason: "checkout recovery persistence failed"}, e
		}
		attempts[index].sessionID = recovered.ID
	}
	for _, attempt := range attempts {
		if err = s.expireCheckout(ctx, attempt.sessionID); err != nil {
			return contracts.CloseResult{Status: contracts.CloseStatusPending, PendingReason: "checkout session unresolved"}, err
		}
	}
	for _, sub := range list.Data {
		if subscriptionTerminal(sub.Status) {
			continue
		}
		var closed subscription
		if err = s.providerRequest(ctx, "DELETE", "/subscriptions/"+url.PathEscape(sub.ID), nil, "serenity-cancel-"+sub.ID, &closed); err != nil {
			return contracts.CloseResult{Status: contracts.CloseStatusPending, PendingReason: "subscription cancellation unresolved"}, err
		}
		if closed.Status != "canceled" {
			return contracts.CloseResult{Status: contracts.CloseStatusPending, PendingReason: "subscription cancellation incomplete"}, nil
		}
	}
	// Re-list after cancellation. This closes the race where a completed
	// checkout creates a subscription after the first provider snapshot.
	final, err := s.listSubscriptions(ctx, customer)
	if err != nil {
		return contracts.CloseResult{Status: contracts.CloseStatusPending, PendingReason: "provider closure recheck unavailable"}, err
	}
	for _, sub := range final.Data {
		if !subscriptionTerminal(sub.Status) {
			return contracts.CloseResult{Status: contracts.CloseStatusPending, PendingReason: "provider still has an active subscription"}, nil
		}
	}
	if err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if _, e := tx.ExecContext(ctx, `UPDATE subscriptions SET status='canceled',grace_until=NULL WHERE account_id=? AND status NOT IN ('canceled','incomplete_expired')`, account); e != nil {
			return e
		}
		if _, e := tx.ExecContext(ctx, `DELETE FROM checkout_attempts WHERE account_id=?`, account); e != nil {
			return e
		}
		_, e := tx.ExecContext(ctx, `UPDATE accounts SET plan_id='free',plan_version=1 WHERE id=?`, account)
		return e
	}); err != nil {
		return contracts.CloseResult{Status: contracts.CloseStatusPending, PendingReason: "local closure commit failed"}, err
	}
	return contracts.CloseResult{Status: contracts.CloseStatusClosed}, nil
}

// CloseBillingAccount closes all provider state for an account already marked
// deleting. It is safe to retry after provider success or a local timeout.

func (s *Service) CloseBillingAccount(ctx context.Context, account string) (contracts.CloseResult, error) {
	return s.closeBilling(ctx, account, true)
}

// CancelAccount cancels subscriptions attached to this account and is kept for
// callers that predate the deletion-safe closure contract.

func (s *Service) CancelAccount(ctx context.Context, account string) error {
	result, err := s.closeBilling(ctx, account, false)
	if err != nil {
		return err
	}
	if result.Status != contracts.CloseStatusClosed {
		return fmt.Errorf("%w: %s", contracts.ErrBillingProviderAmbiguous, result.PendingReason)
	}
	return nil
}

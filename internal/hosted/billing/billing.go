// Package billing reconciles hosted entitlements from Stripe, never return URLs.
package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/identity"
	"github.com/sirerun/serenity/internal/hosted/store"
	"github.com/sirerun/serenity/internal/hosted/testhooks"
)

const APIVersion = "2025-06-30.basil"

type Config struct {
	SecretKey, WebhookSecret, BuilderPrice, ScalePrice, Origin string
	Client                                                     *http.Client
	BaseURL                                                    string
}
type Service struct {
	Store    *store.Store
	Identity *identity.Service
	Config   Config
	mu       sync.Mutex
}

var _ contracts.BillingReconciler = (*Service)(nil)
var _ contracts.BillingCloser = (*Service)(nil)

func (s *Service) request(ctx context.Context, method, path string, form url.Values, key string, result any) error {
	base := s.Config.BaseURL
	if base == "" {
		base = "https://api.stripe.com/v1"
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(s.Config.SecretKey, "")
	req.Header.Set("Stripe-Version", APIVersion)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	client := s.Config.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("billing provider unavailable")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("billing provider status %d", resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(result)
}

func (s *Service) providerRequest(ctx context.Context, method, path string, form url.Values, key string, result any) error {
	if err := s.request(ctx, method, path, form, key, result); err != nil {
		return fmt.Errorf("%w: %v", contracts.ErrBillingProviderUnavailable, err)
	}
	return nil
}

type subscriptionList struct {
	Data    []subscription `json:"data"`
	HasMore bool           `json:"has_more"`
}

func (s *Service) listSubscriptions(ctx context.Context, customer string) (subscriptionList, error) {
	var out subscriptionList
	if customer == "" || !strings.HasPrefix(customer, "cus_") || strings.ContainsAny(customer, "/?#") {
		return out, fmt.Errorf("%w: invalid customer reference", contracts.ErrBillingProviderAmbiguous)
	}
	path := "/subscriptions?" + url.Values{"customer": {customer}, "status": {"all"}, "limit": {"100"}}.Encode()
	if err := s.providerRequest(ctx, "GET", path, nil, "", &out); err != nil {
		return out, err
	}
	if out.HasMore {
		return out, fmt.Errorf("%w: subscription list has more results", contracts.ErrBillingProviderAmbiguous)
	}
	for _, sub := range out.Data {
		if sub.ID == "" || !strings.HasPrefix(sub.ID, "sub_") || strings.ContainsAny(sub.ID, "/?#") || sub.Customer != customer {
			return out, fmt.Errorf("%w: provider returned an unowned subscription", contracts.ErrBillingProviderAmbiguous)
		}
		if len(sub.Items.Data) != 1 {
			return out, fmt.Errorf("%w: unsupported subscription shape", contracts.ErrBillingProviderAmbiguous)
		}
	}
	return out, nil
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

func subscriptionEntitled(status string) bool {
	return status == "active" || status == "trialing" || status == "past_due"
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
	s.mu.Lock()
	defer s.mu.Unlock()
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
			var oldStatus, oldGrace, oldGraceInvoice, oldPeriodStart, oldPeriodEnd string
			_ = tx.QueryRowContext(ctx, `SELECT status,COALESCE(grace_until,''),COALESCE(grace_invoice_id,''),COALESCE(current_period_start,''),COALESCE(current_period_end,'') FROM subscriptions WHERE id=? AND account_id=?`, sub.ID, accountID).Scan(&oldStatus, &oldGrace, &oldGraceInvoice, &oldPeriodStart, &oldPeriodEnd)
			periodStart := time.Unix(item.CurrentPeriodStart, 0).UTC()
			anchor := periodStart
			invoice := ""
			if sub.Status == "past_due" {
				failure := failures[sub.ID]
				anchor, invoice = failure.at, failure.invoice
			}
			newPeriodStart := store.Stamp(periodStart)
			newPeriodEnd := store.Stamp(time.Unix(item.CurrentPeriodEnd, 0))
			periodChanged := oldPeriodStart != "" && oldPeriodStart != newPeriodStart
			if periodChanged {
				if e = recordWindowClosed(ctx, tx, accountID, sub.ID, oldStatus, oldPeriodStart, oldPeriodEnd, oldGrace); e != nil {
					return e
				}
			}
			grace, graceInvoice := graceDeadline(oldStatus, oldGrace, oldGraceInvoice, periodChanged, sub.Status, invoice, anchor)
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
		if chosen == nil || status != "active" {
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
	result.Eligible = subscriptionEntitled(chosen.Status)
	result.PlanID = s.planForPrice(item.Price.ID)
	result.CurrentWindowStart = time.Unix(item.CurrentPeriodStart, 0).UTC()
	result.CurrentWindowEnd = time.Unix(item.CurrentPeriodEnd, 0).UTC()
	result.Source = "stripe_subscription"
	if chosen.Status == "past_due" {
		var grace string
		_ = s.Store.DB().QueryRowContext(ctx, `SELECT COALESCE(grace_until,'') FROM subscriptions WHERE id=?`, chosen.ID).Scan(&grace)
		if grace != "" {
			result.GraceUntil, _ = time.Parse(time.RFC3339Nano, grace)
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
func (s *Service) customer(ctx context.Context, account string) (string, error) {
	var customer sql.NullString
	var email string
	err := s.Store.DB().QueryRowContext(ctx, `SELECT stripe_customer_id,email FROM accounts WHERE id=? AND status='active'`, account).Scan(&customer, &email)
	if err != nil {
		return "", err
	}
	if customer.Valid && customer.String != "" {
		return customer.String, nil
	}
	var created struct {
		ID string `json:"id"`
	}
	err = s.request(ctx, "POST", "/customers", url.Values{"email": {email}, "metadata[serenity_account]": {account}}, "serenity-customer-"+account, &created)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(created.ID, "cus_") {
		return "", errors.New("invalid billing customer response")
	}
	err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `UPDATE accounts SET stripe_customer_id=? WHERE id=? AND status='active'`, created.ID, account)
		return e
	})
	return created.ID, err
}
func (s *Service) Checkout(ctx context.Context, account, plan string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	price := ""
	switch plan {
	case "builder":
		price = s.Config.BuilderPrice
	case "scale":
		price = s.Config.ScalePrice
	}
	if price == "" {
		return "", errors.New("plan is unavailable")
	}
	var n int
	if err := s.Store.DB().QueryRowContext(ctx, `SELECT count(*) FROM subscriptions WHERE account_id=? AND status NOT IN ('canceled','incomplete_expired')`, account).Scan(&n); err != nil {
		return "", err
	}
	if n > 0 {
		return "", errors.New("manage your existing subscription through billing")
	}
	customer, err := s.customer(ctx, account)
	if err != nil {
		return "", err
	}
	// The provider is authoritative even before subscription webhooks arrive.
	var subscriptions struct {
		Data    []subscription `json:"data"`
		HasMore bool           `json:"has_more"`
	}
	if err = s.request(ctx, "GET", "/subscriptions?"+url.Values{"customer": {customer}, "status": {"all"}, "limit": {"100"}}.Encode(), nil, "", &subscriptions); err != nil {
		return "", err
	}
	if subscriptions.HasMore {
		return "", errors.New("billing reconciliation required")
	}
	for _, sub := range subscriptions.Data {
		if sub.Status != "canceled" && sub.Status != "incomplete_expired" {
			return "", errors.New("manage your existing subscription through billing")
		}
	}
	var attempt, pendingPrice, sessionID, created, requestBody string
	var requestVersion int
	err = s.Store.DB().QueryRowContext(ctx, `SELECT id,price_id,COALESCE(session_id,''),created_at,request_version,COALESCE(request_body,'') FROM checkout_attempts WHERE account_id=?`, account).Scan(&attempt, &pendingPrice, &sessionID, &created, &requestVersion, &requestBody)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	var result checkoutSession
	var previousAttempt string
	if sessionID != "" {
		if err = s.request(ctx, "GET", "/checkout/sessions/"+url.PathEscape(sessionID), nil, "", &result); err != nil {
			return "", err
		}
		if result.ID != sessionID {
			return "", fmt.Errorf("%w: checkout session identity mismatch", contracts.ErrBillingProviderAmbiguous)
		}
		if requestVersion == 1 {
			if err = validateCheckoutIdentity(result, account, customer, attempt); err != nil {
				return "", err
			}
		}
		if result.Status == "open" {
			at, parseErr := time.Parse(time.RFC3339Nano, created)
			if parseErr != nil || time.Since(at) > 23*time.Hour {
				return "", errors.New("unresolved checkout requires billing reconciliation")
			}
			if pendingPrice != price {
				return "", errors.New("retry your existing checkout plan first")
			}
			if !strings.HasPrefix(result.URL, "https://checkout.stripe.com/") {
				return "", errors.New("invalid checkout URL")
			}
			return result.URL, nil
		}
		if result.Status != "expired" && result.Status != "complete" {
			return "", fmt.Errorf("%w: checkout status unresolved", contracts.ErrBillingProviderAmbiguous)
		}
		if result.Status == "complete" {
			if err = s.request(ctx, "GET", "/subscriptions?"+url.Values{"customer": {customer}, "status": {"all"}, "limit": {"100"}}.Encode(), nil, "", &subscriptions); err != nil {
				return "", err
			}
			if subscriptions.HasMore || len(subscriptions.Data) == 0 {
				return "", errors.New("checkout reconciliation required")
			}
			for _, sub := range subscriptions.Data {
				if sub.Status != "canceled" && sub.Status != "incomplete_expired" {
					return "", errors.New("manage your existing subscription through billing")
				}
			}
		}
		previousAttempt = attempt
		attempt = ""
	} else if attempt != "" {
		if requestVersion != 1 || requestBody == "" {
			return "", fmt.Errorf("%w: legacy checkout creation outcome unresolved", contracts.ErrBillingProviderAmbiguous)
		}
		var found bool
		result, found, err = s.discoverCheckoutAttempt(ctx, account, customer, attempt)
		if err != nil {
			return "", err
		}
		if !found {
			return "", fmt.Errorf("%w: checkout session not yet discoverable", contracts.ErrBillingProviderAmbiguous)
		}
		if err = s.saveRecoveredCheckoutID(ctx, account, attempt, result.ID); err != nil {
			return "", err
		}
		sessionID = result.ID
		if result.Status == "open" {
			if pendingPrice != price {
				return "", errors.New("finish or let your existing checkout expire before choosing another plan")
			}
			if !strings.HasPrefix(result.URL, "https://checkout.stripe.com/") {
				return "", errors.New("invalid checkout URL")
			}
			return result.URL, nil
		}
		if result.Status != "expired" && result.Status != "complete" {
			return "", errors.New("checkout reconciliation required")
		}
		// Recheck after retrieving a completed checkout: it could have completed
		// between the first subscription query and the session retrieval.
		if result.Status == "complete" {
			if err = s.request(ctx, "GET", "/subscriptions?"+url.Values{"customer": {customer}, "status": {"all"}, "limit": {"100"}}.Encode(), nil, "", &subscriptions); err != nil {
				return "", err
			}
			if subscriptions.HasMore || len(subscriptions.Data) == 0 {
				return "", errors.New("checkout reconciliation required")
			}
			for _, sub := range subscriptions.Data {
				if sub.Status != "canceled" && sub.Status != "incomplete_expired" {
					return "", errors.New("manage your existing subscription through billing")
				}
			}
		}
		previousAttempt = attempt
		attempt = ""
	}
	if attempt == "" {
		attempt, pendingPrice, created = store.ID(), price, store.Stamp(time.Now())
		form := url.Values{"mode": {"subscription"}, "customer": {customer}, "client_reference_id": {account}, "line_items[0][price]": {price}, "line_items[0][quantity]": {"1"}, "subscription_data[metadata][serenity_account]": {account}, "subscription_data[metadata][serenity_attempt]": {attempt}, "metadata[serenity_account]": {account}, "metadata[serenity_attempt]": {attempt}, "success_url": {s.Config.Origin + "/billing?checkout=returned"}, "cancel_url": {s.Config.Origin + "/billing"}}
		requestBody = form.Encode()
		err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
			if previousAttempt != "" {
				result, e := tx.ExecContext(ctx, `DELETE FROM checkout_attempts WHERE account_id=? AND id=?`, account, previousAttempt)
				if e != nil {
					return e
				}
				changed, e := result.RowsAffected()
				if e != nil {
					return e
				}
				if changed != 1 {
					return fmt.Errorf("%w: prior checkout attempt changed", contracts.ErrBillingProviderAmbiguous)
				}
			}
			_, e := tx.ExecContext(ctx, `INSERT INTO checkout_attempts(account_id,id,price_id,created_at,request_version,request_body) VALUES(?,?,?,?,1,?)`, account, attempt, price, created, requestBody)
			return e
		})
		if err != nil {
			return "", err
		}
	} else {
		at, e := time.Parse(time.RFC3339Nano, created)
		if e != nil || time.Since(at) > 23*time.Hour {
			return "", errors.New("unresolved checkout requires billing reconciliation")
		}
		if pendingPrice != price {
			return "", errors.New("retry your existing checkout plan first")
		}
	}
	form, err := url.ParseQuery(requestBody)
	if err != nil {
		return "", fmt.Errorf("%w: stored checkout request is invalid", contracts.ErrBillingProviderAmbiguous)
	}
	err = s.request(ctx, "POST", "/checkout/sessions", form, "serenity-checkout-"+attempt, &result)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(result.ID, "cs_") || !strings.HasPrefix(result.URL, "https://checkout.stripe.com/") {
		return "", errors.New("invalid checkout response")
	}
	_, err = s.Store.DB().ExecContext(ctx, `UPDATE checkout_attempts SET session_id=? WHERE account_id=? AND id=?`, result.ID, account, attempt)
	if err != nil {
		return "", err
	}
	return result.URL, nil
}

// Portal returns a Stripe-hosted billing portal session URL. The provider's
// portal configuration controls permitted price changes, proration, scheduled
// downgrades and cancellations; creating a session does not configure them.
// A scheduled price downgrade is distinct from cancel_at_period_end: entitlement
// follows the current authoritative price until Stripe applies the new price.
// Deployment must qualify the portal configuration and resulting lifecycle.
func (s *Service) Portal(ctx context.Context, account string) (string, error) {
	customer, err := s.customer(ctx, account)
	if err != nil {
		return "", err
	}
	var result struct {
		URL string `json:"url"`
	}
	err = s.request(ctx, "POST", "/billing_portal/sessions", url.Values{"customer": {customer}, "return_url": {s.Config.Origin + "/billing"}}, "", &result)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(result.URL, "https://billing.stripe.com/") {
		return "", errors.New("invalid billing portal URL")
	}
	return result.URL, nil
}
func VerifySignature(body []byte, header, secret string, now time.Time) bool {
	if secret == "" {
		return false
	}
	var timestamp string
	var signatures []string
	for _, part := range strings.Split(header, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		if key == "t" {
			if timestamp != "" {
				return false
			}
			timestamp = value
		}
		if key == "v1" {
			signatures = append(signatures, value)
		}
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	delta := now.Sub(time.Unix(seconds, 0))
	if delta > 5*time.Minute || delta < -5*time.Minute {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(body)
	want := mac.Sum(nil)
	for _, value := range signatures {
		got, e := hex.DecodeString(value)
		if e == nil && hmac.Equal(got, want) {
			return true
		}
	}
	return false
}

type subscription struct {
	ID                string          `json:"id"`
	Customer          string          `json:"customer"`
	Status            string          `json:"status"`
	LatestInvoice     json.RawMessage `json:"latest_invoice"`
	CancelAtPeriodEnd bool            `json:"cancel_at_period_end"`
	Items             struct {
		Data []struct {
			CurrentPeriodStart int64 `json:"current_period_start"`
			CurrentPeriodEnd   int64 `json:"current_period_end"`
			Price              struct {
				ID string `json:"id"`
			} `json:"price"`
		} `json:"data"`
	} `json:"items"`
}

type invoiceFailureEvent struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Created int64  `json:"created"`
	Data    struct {
		Object struct {
			ID           string `json:"id"`
			Subscription string `json:"subscription"`
			Parent       struct {
				SubscriptionDetails struct {
					Subscription string `json:"subscription"`
				} `json:"subscription_details"`
			} `json:"parent"`
		} `json:"object"`
	} `json:"data"`
}

type invoiceFailureEventList struct {
	Data    []invoiceFailureEvent `json:"data"`
	HasMore bool                  `json:"has_more"`
}

type checkoutSession struct {
	ID                string            `json:"id"`
	URL               string            `json:"url"`
	Status            string            `json:"status"`
	Mode              string            `json:"mode"`
	Customer          string            `json:"customer"`
	ClientReferenceID string            `json:"client_reference_id"`
	Metadata          map[string]string `json:"metadata"`
}

type checkoutSessionList struct {
	Data    []checkoutSession `json:"data"`
	HasMore bool              `json:"has_more"`
}

func validateCheckoutIdentity(session checkoutSession, account, customer, attempt string) error {
	if !strings.HasPrefix(session.ID, "cs_") || strings.ContainsAny(session.ID, "/?#") ||
		session.Customer != customer || session.ClientReferenceID != account || session.Mode != "subscription" ||
		session.Metadata["serenity_account"] != account || session.Metadata["serenity_attempt"] != attempt {
		return fmt.Errorf("%w: checkout session identity mismatch", contracts.ErrBillingProviderAmbiguous)
	}
	return nil
}

func (s *Service) discoverCheckoutAttempt(ctx context.Context, account, customer, attempt string) (checkoutSession, bool, error) {
	var found checkoutSession
	query := url.Values{"customer": {customer}, "limit": {"100"}}
	var cursor string
	for page := 0; page < 100; page++ {
		if cursor != "" {
			query.Set("starting_after", cursor)
		}
		var sessions checkoutSessionList
		if err := s.providerRequest(ctx, "GET", "/checkout/sessions?"+query.Encode(), nil, "", &sessions); err != nil {
			return checkoutSession{}, false, err
		}
		if len(sessions.Data) == 0 && sessions.HasMore {
			return checkoutSession{}, false, fmt.Errorf("%w: malformed checkout pagination", contracts.ErrBillingProviderAmbiguous)
		}
		for _, session := range sessions.Data {
			if session.ID == "" || !strings.HasPrefix(session.ID, "cs_") {
				return checkoutSession{}, false, fmt.Errorf("%w: malformed checkout session list", contracts.ErrBillingProviderAmbiguous)
			}
			if session.Metadata["serenity_attempt"] != attempt {
				continue
			}
			if err := validateCheckoutIdentity(session, account, customer, attempt); err != nil {
				return checkoutSession{}, false, err
			}
			if found.ID != "" && found.ID != session.ID {
				return checkoutSession{}, false, fmt.Errorf("%w: duplicate checkout sessions for one attempt", contracts.ErrBillingProviderAmbiguous)
			}
			found = session
		}
		if !sessions.HasMore {
			return found, found.ID != "", nil
		}
		last := sessions.Data[len(sessions.Data)-1].ID
		if last == "" || last == cursor {
			return checkoutSession{}, false, fmt.Errorf("%w: malformed checkout cursor", contracts.ErrBillingProviderAmbiguous)
		}
		cursor = last
		if page == 99 {
			return checkoutSession{}, false, fmt.Errorf("%w: checkout history exceeds page limit", contracts.ErrBillingProviderAmbiguous)
		}
	}
	return checkoutSession{}, false, fmt.Errorf("%w: incomplete checkout history", contracts.ErrBillingProviderAmbiguous)
}

func (s *Service) saveRecoveredCheckoutID(ctx context.Context, account, attempt, session string) error {
	result, err := s.Store.DB().ExecContext(ctx, `UPDATE checkout_attempts SET session_id=? WHERE account_id=? AND id=? AND (session_id IS NULL OR session_id='')`, session, account, attempt)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 1 {
		return nil
	}
	var current string
	if err = s.Store.DB().QueryRowContext(ctx, `SELECT COALESCE(session_id,'') FROM checkout_attempts WHERE account_id=? AND id=?`, account, attempt).Scan(&current); err != nil || current != session {
		return fmt.Errorf("%w: checkout attempt changed during recovery", contracts.ErrBillingProviderAmbiguous)
	}
	return nil
}

func invoiceID(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", fmt.Errorf("%w: past_due subscription has no latest invoice", contracts.ErrBillingProviderAmbiguous)
	}
	var id string
	if json.Unmarshal(raw, &id) == nil && id != "" {
		return id, nil
	}
	var expanded struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &expanded); err != nil || expanded.ID == "" {
		return "", fmt.Errorf("%w: invalid latest invoice", contracts.ErrBillingProviderAmbiguous)
	}
	return expanded.ID, nil
}

func invoiceSubscription(raw json.RawMessage) string {
	var invoice struct {
		Subscription string `json:"subscription"`
		Parent       struct {
			SubscriptionDetails struct {
				Subscription string `json:"subscription"`
			} `json:"subscription_details"`
		} `json:"parent"`
	}
	if json.Unmarshal(raw, &invoice) != nil {
		return ""
	}
	if invoice.Subscription != "" {
		return invoice.Subscription
	}
	return invoice.Parent.SubscriptionDetails.Subscription
}

// failureEvidence resolves the first payment-failure event for the exact
// latest invoice. Both webhook and reconciliation paths use this provider
// identity and persist it before calculating grace; event delivery time and
// local processing time never become competing anchors.
func (s *Service) failureEvidence(ctx context.Context, accountID string, sub subscription) (time.Time, error) {
	id, err := invoiceID(sub.LatestInvoice)
	if err != nil {
		return time.Time{}, err
	}
	if !strings.HasPrefix(id, "in_") || strings.ContainsAny(id, "/?#") {
		return time.Time{}, fmt.Errorf("%w: invalid invoice reference", contracts.ErrBillingProviderAmbiguous)
	}
	var eventID, firstFailed string
	err = s.Store.DB().QueryRowContext(ctx, `SELECT event_id,first_failed_at FROM billing_failures WHERE account_id=? AND subscription_id=? AND invoice_id=?`, accountID, sub.ID, id).Scan(&eventID, &firstFailed)
	if err == nil {
		at, parseErr := time.Parse(time.RFC3339Nano, firstFailed)
		if parseErr != nil || !strings.HasPrefix(eventID, "evt_") {
			return time.Time{}, fmt.Errorf("%w: invalid stored payment-failure evidence", contracts.ErrBillingProviderAmbiguous)
		}
		return at.UTC(), nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, err
	}
	var invoice struct {
		ID       string `json:"id"`
		Customer string `json:"customer"`
		Created  int64  `json:"created"`
	}
	var raw json.RawMessage
	if err = s.providerRequest(ctx, "GET", "/invoices/"+url.PathEscape(id), nil, "", &raw); err != nil {
		return time.Time{}, err
	}
	if err = json.Unmarshal(raw, &invoice); err != nil || invoice.ID != id || invoice.Customer != sub.Customer || invoiceSubscription(raw) != sub.ID || invoice.Created <= 0 {
		return time.Time{}, fmt.Errorf("%w: invoice identity mismatch", contracts.ErrBillingProviderAmbiguous)
	}
	// Stripe only retains listable events for 30 days. If this invoice is older
	// and its exact failure was not already persisted locally, its first-failure
	// time can no longer be proved, so reconciliation fails closed.
	if time.Unix(invoice.Created, 0).Before(time.Now().UTC().Add(-30 * 24 * time.Hour)) {
		return time.Time{}, fmt.Errorf("%w: invoice failure history is outside provider retention", contracts.ErrBillingProviderAmbiguous)
	}
	first := time.Time{}
	firstID := ""
	var cursor string
	for page := 0; page < 100; page++ {
		query := url.Values{"type": {"invoice.payment_failed"}, "created[gte]": {strconv.FormatInt(invoice.Created, 10)}, "limit": {"100"}}
		if cursor != "" {
			query.Set("starting_after", cursor)
		}
		var events invoiceFailureEventList
		if err = s.providerRequest(ctx, "GET", "/events?"+query.Encode(), nil, "", &events); err != nil {
			return time.Time{}, err
		}
		if len(events.Data) == 0 && events.HasMore {
			return time.Time{}, fmt.Errorf("%w: malformed invoice event pagination", contracts.ErrBillingProviderAmbiguous)
		}
		for _, event := range events.Data {
			if event.ID == "" || !strings.HasPrefix(event.ID, "evt_") || event.Type != "invoice.payment_failed" || event.Created <= 0 {
				return time.Time{}, fmt.Errorf("%w: malformed invoice failure event", contracts.ErrBillingProviderAmbiguous)
			}
			if event.Data.Object.ID != id {
				continue
			}
			failureSub := event.Data.Object.Subscription
			if failureSub == "" {
				failureSub = event.Data.Object.Parent.SubscriptionDetails.Subscription
			}
			if failureSub != sub.ID {
				return time.Time{}, fmt.Errorf("%w: invoice failure subscription mismatch", contracts.ErrBillingProviderAmbiguous)
			}
			at := time.Unix(event.Created, 0).UTC()
			if first.IsZero() || at.Before(first) || at.Equal(first) && event.ID < firstID {
				first, firstID = at, event.ID
			}
		}
		if !events.HasMore {
			break
		}
		last := events.Data[len(events.Data)-1].ID
		if last == cursor || last == "" {
			return time.Time{}, fmt.Errorf("%w: malformed invoice event cursor", contracts.ErrBillingProviderAmbiguous)
		}
		cursor = last
		if page == 99 {
			return time.Time{}, fmt.Errorf("%w: invoice failure history exceeds page limit", contracts.ErrBillingProviderAmbiguous)
		}
	}
	if first.IsZero() {
		return time.Time{}, fmt.Errorf("%w: no failure event found for latest invoice", contracts.ErrBillingProviderAmbiguous)
	}
	err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `INSERT INTO billing_failures(account_id,subscription_id,invoice_id,event_id,first_failed_at) VALUES(?,?,?,?,?) ON CONFLICT(subscription_id,invoice_id) DO UPDATE SET event_id=CASE WHEN excluded.first_failed_at<first_failed_at OR (excluded.first_failed_at=first_failed_at AND excluded.event_id<event_id) THEN excluded.event_id ELSE event_id END,first_failed_at=MIN(first_failed_at,excluded.first_failed_at)`, accountID, sub.ID, id, firstID, store.Stamp(first))
		return e
	})
	if err != nil {
		return time.Time{}, err
	}
	return first.UTC(), nil
}

func (s *Service) Webhook(ctx context.Context, body []byte, signature string) error {
	if !VerifySignature(body, signature, s.Config.WebhookSecret, time.Now()) {
		return errors.New("invalid webhook signature")
	}
	var event struct {
		ID, Type string
		Created  int64
		Data     struct{ Object json.RawMessage }
	}
	if err := json.Unmarshal(body, &event); err != nil {
		return err
	}
	if event.ID == "" {
		return errors.New("missing event id")
	}
	if event.Created <= 0 {
		return errors.New("missing event creation time")
	}
	// Serializing fetch + commit prevents a slower old fetch overwriting newer state.
	s.mu.Lock()
	defer s.mu.Unlock()
	var processed sql.NullString
	err := s.Store.DB().QueryRowContext(ctx, `SELECT processed_at FROM stripe_events WHERE id=?`, event.ID).Scan(&processed)
	if err == nil && processed.Valid {
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `INSERT INTO stripe_events(id,received_at) VALUES(?,?) ON CONFLICT DO NOTHING`, event.ID, store.Stamp(time.Now()))
		return e
	}); err != nil {
		return err
	}
	var object struct {
		ID           string `json:"id"`
		Subscription string `json:"subscription"`
		Parent       struct {
			SubscriptionDetails struct {
				Subscription string `json:"subscription"`
			} `json:"subscription_details"`
		} `json:"parent"`
	}
	if err = json.Unmarshal(event.Data.Object, &object); err != nil {
		return err
	}
	id := object.Subscription
	if strings.HasPrefix(event.Type, "customer.subscription.") {
		id = object.ID
	}
	if id == "" {
		id = object.Parent.SubscriptionDetails.Subscription
	}
	if id != "" {
		if !strings.HasPrefix(id, "sub_") || strings.ContainsAny(id, "/?#") {
			return errors.New("invalid subscription reference")
		}
		var sub subscription
		if err = s.request(ctx, "GET", "/subscriptions/"+id, nil, "", &sub); err != nil {
			return err
		}
		if sub.ID != id || len(sub.Items.Data) != 1 {
			return errors.New("unsupported subscription shape")
		}
		item := sub.Items.Data[0]
		plan := ""
		if item.Price.ID == s.Config.BuilderPrice {
			plan = "builder"
		}
		if item.Price.ID == s.Config.ScalePrice {
			plan = "scale"
		}
		if plan == "" {
			return errors.New("subscription price is not a Serenity price")
		}
		var account, accountStatus string
		e := s.Store.DB().QueryRowContext(ctx, `SELECT id,status FROM accounts WHERE stripe_customer_id=?`, sub.Customer).Scan(&account, &accountStatus)
		if errors.Is(e, sql.ErrNoRows) {
			// The account may have completed deletion after the provider emitted
			// this event. Acknowledge it without creating an entitlement.
			return s.Store.Transaction(ctx, func(tx *sql.Tx) error {
				_, e := tx.ExecContext(ctx, `UPDATE stripe_events SET processed_at=? WHERE id=?`, store.Stamp(time.Now()), event.ID)
				return e
			})
		}
		if e != nil {
			return e
		}
		anchor := time.Unix(item.CurrentPeriodStart, 0).UTC()
		invoice := ""
		if sub.Status == "past_due" {
			anchor, err = s.failureEvidence(ctx, account, sub)
			if err != nil {
				return err
			}
			invoice, err = invoiceID(sub.LatestInvoice)
			if err != nil {
				return err
			}
		}
		err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
			var oldStatus, oldGrace, oldGraceInvoice, oldPeriodStart, oldPeriodEnd string
			_ = tx.QueryRowContext(ctx, `SELECT status,COALESCE(grace_until,''),COALESCE(grace_invoice_id,''),COALESCE(current_period_start,''),COALESCE(current_period_end,'') FROM subscriptions WHERE id=?`, sub.ID).Scan(&oldStatus, &oldGrace, &oldGraceInvoice, &oldPeriodStart, &oldPeriodEnd)
			newPeriodStart := store.Stamp(time.Unix(item.CurrentPeriodStart, 0))
			newPeriodEnd := store.Stamp(time.Unix(item.CurrentPeriodEnd, 0))
			periodChanged := oldPeriodStart != "" && oldPeriodStart != newPeriodStart
			if periodChanged {
				if e = recordWindowClosed(ctx, tx, account, sub.ID, oldStatus, oldPeriodStart, oldPeriodEnd, oldGrace); e != nil {
					return e
				}
			}
			grace, graceInvoice := graceDeadline(oldStatus, oldGrace, oldGraceInvoice, periodChanged, sub.Status, invoice, anchor)
			_, e = tx.ExecContext(ctx, `INSERT INTO subscriptions(id,account_id,price_id,plan_id,status,current_period_start,current_period_end,cancel_at_period_end,grace_until,grace_invoice_id) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET price_id=excluded.price_id,plan_id=excluded.plan_id,status=excluded.status,current_period_start=excluded.current_period_start,current_period_end=excluded.current_period_end,cancel_at_period_end=excluded.cancel_at_period_end,grace_until=excluded.grace_until,grace_invoice_id=excluded.grace_invoice_id`, sub.ID, account, item.Price.ID, plan, sub.Status, newPeriodStart, newPeriodEnd, sub.CancelAtPeriodEnd, nullableString(grace), nullableString(graceInvoice))
			if e != nil {
				return e
			}
			if accountStatus == "active" {
				_, e = tx.ExecContext(ctx, `UPDATE accounts SET plan_id=?,plan_version=1 WHERE id=?`, plan, account)
				if e != nil {
					return e
				}
			} else if _, e = tx.ExecContext(ctx, `UPDATE accounts SET plan_id='free',plan_version=1 WHERE id=?`, account); e != nil {
				return e
			}
			_, e = tx.ExecContext(ctx, `UPDATE stripe_events SET processed_at=? WHERE id=?`, store.Stamp(time.Now()), event.ID)
			return e
		})
		return err
	}
	return s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `UPDATE stripe_events SET processed_at=? WHERE id=?`, store.Stamp(time.Now()), event.ID)
		return e
	})
}
func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path == "/billing/webhook" {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			http.Error(w, "Invalid webhook", http.StatusBadRequest)
			return
		}
		if !VerifySignature(body, r.Header.Get("Stripe-Signature"), s.Config.WebhookSecret, time.Now()) {
			http.Error(w, "Invalid signature", http.StatusBadRequest)
			return
		}
		if err = s.Webhook(r.Context(), body, r.Header.Get("Stripe-Signature")); err != nil {
			http.Error(w, "Webhook processing unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(200)
		return
	}
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cookie, err := r.Cookie("serenity_session")
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	session, err := s.Identity.Session(r.Context(), cookie.Value)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if err = r.ParseForm(); err != nil || !identity.CheckCSRF(session, r.FormValue("csrf")) || r.Header.Get("Sec-Fetch-Site") == "cross-site" || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != s.Config.Origin) {
		http.Error(w, "Invalid form", http.StatusForbidden)
		return
	}
	var target string
	switch r.URL.Path {
	case "/billing/checkout":
		target, err = s.Checkout(r.Context(), session.AccountID, r.FormValue("plan"))
	case "/billing/portal":
		target, err = s.Portal(r.Context(), session.AccountID)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "Billing is temporarily unavailable. Your existing memory remains accessible.", http.StatusServiceUnavailable)
		return
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

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
	s.mu.Lock()
	defer s.mu.Unlock()
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

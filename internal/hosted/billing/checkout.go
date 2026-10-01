// Package billing reconciles hosted entitlements from Stripe, never return URLs.
package billing

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"net/url"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/store"
)

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
	release, err := s.locks.acquire(ctx, "account:"+account)
	if err != nil {
		return "", err
	}
	defer release()
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
	var accountStatus string
	if err := s.Store.DB().QueryRowContext(ctx, `SELECT status FROM accounts WHERE id=? AND status='active'`, account).Scan(&accountStatus); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	allowed, retryAfter, err := s.checkoutLimiter.charge(ctx, account, time.Now().UTC())
	if err != nil {
		return "", err
	}
	if !allowed {
		return "", &CheckoutRateLimitError{retryAfter: retryAfter}
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

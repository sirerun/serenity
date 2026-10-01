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

	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/store"
)

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
	// Serialize duplicate delivery by event ID. The account key is resolved from
	// a provider-owned customer ID below, then locked before the authoritative
	// subscription refetch and projection commit.
	releaseEvent, err := s.locks.acquire(ctx, "event:"+event.ID)
	if err != nil {
		return err
	}
	defer releaseEvent()
	var processed sql.NullString
	err = s.Store.DB().QueryRowContext(ctx, `SELECT processed_at FROM stripe_events WHERE id=?`, event.ID).Scan(&processed)
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
		if err = s.providerRequest(ctx, "GET", "/subscriptions/"+url.PathEscape(id), nil, "", &sub); err != nil {
			return err
		}
		if sub.ID != id || !strings.HasPrefix(sub.Customer, "cus_") || strings.ContainsAny(sub.Customer, "/?#") {
			return contracts.ErrBillingProviderAmbiguous
		}
		var account, accountStatus, boundCustomer string
		e := s.Store.DB().QueryRowContext(ctx, `SELECT id,status,stripe_customer_id FROM accounts WHERE stripe_customer_id=?`, sub.Customer).Scan(&account, &accountStatus, &boundCustomer)
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
		releaseAccount, lockErr := s.locks.acquire(ctx, "account:"+account)
		if lockErr != nil {
			return lockErr
		}
		defer releaseAccount()
		var currentCustomer, currentStatus string
		if e = s.Store.DB().QueryRowContext(ctx, `SELECT COALESCE(stripe_customer_id,''),status FROM accounts WHERE id=?`, account).Scan(&currentCustomer, &currentStatus); e != nil {
			return e
		}
		if currentCustomer != boundCustomer || currentCustomer != sub.Customer {
			return contracts.ErrBillingProviderAmbiguous
		}
		accountStatus = currentStatus
		// This second provider read is the only subscription snapshot used for
		// projection. The first snapshot established only the trusted lock key.
		sub = subscription{}
		if err = s.providerRequest(ctx, "GET", "/subscriptions/"+url.PathEscape(id), nil, "", &sub); err != nil {
			return err
		}
		if sub.ID != id || sub.Customer != currentCustomer || len(sub.Items.Data) != 1 {
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
			old, readErr := s.readPriorSubscription(ctx, tx, sub.ID, account)
			if readErr != nil {
				return readErr
			}
			newPeriodStart := store.Stamp(time.Unix(item.CurrentPeriodStart, 0))
			newPeriodEnd := store.Stamp(time.Unix(item.CurrentPeriodEnd, 0))
			periodChanged := old.periodStart != "" && old.periodStart != newPeriodStart
			if periodChanged {
				if e = recordWindowClosed(ctx, tx, account, sub.ID, old.status, old.periodStart, old.periodEnd, old.grace); e != nil {
					return e
				}
			}
			grace, graceInvoice := graceDeadline(old.status, old.grace, old.graceInvoice, periodChanged, sub.Status, invoice, anchor)
			eligible, accessErr := effectiveSubscriptionAccess(sub.Status, time.Unix(item.CurrentPeriodEnd, 0).UTC(), grace, time.Now().UTC())
			if accessErr != nil {
				return accessErr
			}
			_, e = tx.ExecContext(ctx, `INSERT INTO subscriptions(id,account_id,price_id,plan_id,status,current_period_start,current_period_end,cancel_at_period_end,grace_until,grace_invoice_id) VALUES(?,?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET price_id=excluded.price_id,plan_id=excluded.plan_id,status=excluded.status,current_period_start=excluded.current_period_start,current_period_end=excluded.current_period_end,cancel_at_period_end=excluded.cancel_at_period_end,grace_until=excluded.grace_until,grace_invoice_id=excluded.grace_invoice_id`, sub.ID, account, item.Price.ID, plan, sub.Status, newPeriodStart, newPeriodEnd, sub.CancelAtPeriodEnd, nullableString(grace), nullableString(graceInvoice))
			if e != nil {
				return e
			}
			accountPlan := "free"
			if accountStatus == "active" && eligible {
				accountPlan = plan
			}
			if _, e = tx.ExecContext(ctx, `UPDATE accounts SET plan_id=?,plan_version=1 WHERE id=?`, accountPlan, account); e != nil {
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

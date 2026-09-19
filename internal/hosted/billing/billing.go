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

	"github.com/sirerun/serenity/internal/hosted/identity"
	"github.com/sirerun/serenity/internal/hosted/store"
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
	var attempt, pendingPrice, sessionID, created string
	err = s.Store.DB().QueryRowContext(ctx, `SELECT id,price_id,COALESCE(session_id,''),created_at FROM checkout_attempts WHERE account_id=?`, account).Scan(&attempt, &pendingPrice, &sessionID, &created)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	type checkout struct {
		ID     string `json:"id"`
		URL    string `json:"url"`
		Status string `json:"status"`
	}
	var result checkout
	if sessionID != "" {
		if err = s.request(ctx, "GET", "/checkout/sessions/"+url.PathEscape(sessionID), nil, "", &result); err != nil {
			return "", err
		}
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
		attempt = ""
	}
	if attempt == "" {
		attempt, pendingPrice, created = store.ID(), price, store.Stamp(time.Now())
		_, err = s.Store.DB().ExecContext(ctx, `INSERT INTO checkout_attempts(account_id,id,price_id,created_at) VALUES(?,?,?,?) ON CONFLICT(account_id) DO UPDATE SET id=excluded.id,price_id=excluded.price_id,created_at=excluded.created_at,session_id=NULL`, account, attempt, price, created)
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
	err = s.request(ctx, "POST", "/checkout/sessions", url.Values{"mode": {"subscription"}, "customer": {customer}, "client_reference_id": {account}, "line_items[0][price]": {price}, "line_items[0][quantity]": {"1"}, "subscription_data[metadata][serenity_account]": {account}, "success_url": {s.Config.Origin + "/billing?checkout=returned"}, "cancel_url": {s.Config.Origin + "/billing"}}, "serenity-checkout-"+attempt, &result)
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
	ID                string `json:"id"`
	Customer          string `json:"customer"`
	Status            string `json:"status"`
	CancelAtPeriodEnd bool   `json:"cancel_at_period_end"`
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

func (s *Service) Webhook(ctx context.Context, body []byte, signature string) error {
	if !VerifySignature(body, signature, s.Config.WebhookSecret, time.Now()) {
		return errors.New("invalid webhook signature")
	}
	var event struct {
		ID, Type string
		Data     struct{ Object json.RawMessage }
	}
	if err := json.Unmarshal(body, &event); err != nil {
		return err
	}
	if event.ID == "" {
		return errors.New("missing event id")
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
		err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
			var account string
			e := tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE stripe_customer_id=? AND status='active'`, sub.Customer).Scan(&account)
			if e != nil {
				return e
			}
			_, e = tx.ExecContext(ctx, `INSERT INTO subscriptions(id,account_id,price_id,plan_id,status,current_period_start,current_period_end,cancel_at_period_end) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET grace_until=CASE WHEN excluded.status='past_due' AND subscriptions.status IN ('active','trialing') THEN ? WHEN excluded.status='past_due' AND subscriptions.status='past_due' THEN subscriptions.grace_until ELSE NULL END,price_id=excluded.price_id,plan_id=excluded.plan_id,status=excluded.status,current_period_start=excluded.current_period_start,current_period_end=excluded.current_period_end,cancel_at_period_end=excluded.cancel_at_period_end`, sub.ID, account, item.Price.ID, plan, sub.Status, store.Stamp(time.Unix(item.CurrentPeriodStart, 0)), store.Stamp(time.Unix(item.CurrentPeriodEnd, 0)), sub.CancelAtPeriodEnd, store.Stamp(time.Now().Add(72*time.Hour)))
			if e != nil {
				return e
			}
			_, e = tx.ExecContext(ctx, `UPDATE accounts SET plan_id=?,plan_version=1 WHERE id=?`, plan, account)
			if e != nil {
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

// CancelAccount cancels only subscriptions attached to this authenticated account.
func (s *Service) CancelAccount(ctx context.Context, account string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.Store.DB().QueryContext(ctx, `SELECT id FROM subscriptions WHERE account_id=? AND status NOT IN ('canceled','incomplete_expired')`, account)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	for _, id := range ids {
		if !strings.HasPrefix(id, "sub_") || strings.ContainsAny(id, "/?#") {
			return errors.New("invalid subscription reference")
		}
		var sub subscription
		if err = s.request(ctx, "DELETE", "/subscriptions/"+id, nil, "serenity-cancel-"+id, &sub); err != nil {
			return err
		}
		if sub.Status != "canceled" {
			return errors.New("subscription cancellation incomplete")
		}
	}
	return nil
}

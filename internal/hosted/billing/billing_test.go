package billing_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/billing"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/meter"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func signature(body []byte, secret string, at time.Time) string {
	stamp := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(stamp + "."))
	_, _ = mac.Write(body)
	return "t=" + stamp + ",v1=" + hex.EncodeToString(mac.Sum(nil))
}
func TestWebhookSignature(t *testing.T) {
	body := []byte(`{"id":"evt_test"}`)
	now := time.Now()
	sig := signature(body, "secret", now)
	if !billing.VerifySignature(body, sig, "secret", now) {
		t.Fatal("valid rejected")
	}
	if billing.VerifySignature([]byte("changed"), sig, "secret", now) || billing.VerifySignature(body, sig, "other", now) || billing.VerifySignature(body, sig, "secret", now.Add(6*time.Minute)) {
		t.Fatal("invalid accepted")
	}
}
func TestWebhookReconcilesCurrentStateAndDeduplicates(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	a, err := db.CreateAccount(ctx, "buyer@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Transaction(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_test' WHERE id=?`, a.ID)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	state := "active"
	var requests atomic.Int32
	now := time.Now()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Stripe-Version") != billing.APIVersion {
			t.Error("unpinned API")
		}
		if r.URL.Path != "/subscriptions/sub_test" {
			t.Error(r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"sub_test","customer":"cus_test","status":%q,"items":{"data":[{"price":{"id":"price_builder"},"current_period_start":%d,"current_period_end":%d}]}}`, state, now.Unix(), now.Add(24*time.Hour).Unix())
	}))
	defer provider.Close()
	s := &billing.Service{Store: db, Config: billing.Config{SecretKey: "test", WebhookSecret: "secret", BuilderPrice: "price_builder", ScalePrice: "price_scale", BaseURL: provider.URL}}
	apply := func(id string) {
		body, _ := json.Marshal(map[string]any{"id": id, "type": "customer.subscription.updated", "created": time.Now().Unix(), "data": map[string]any{"object": map[string]any{"id": "sub_test", "status": "stale-untrusted"}}})
		if e := s.Webhook(ctx, body, signature(body, "secret", time.Now())); e != nil {
			t.Fatal(e)
		}
	}
	m := &meter.Meter{Store: db}
	apply("evt_new")
	ent, err := m.Entitlement(ctx, a.ID)
	if err != nil || ent.Plan.ID != "builder" {
		t.Fatalf("entitlement %+v %v", ent, err)
	}
	apply("evt_new")
	if requests.Load() != 1 {
		t.Fatal("duplicate fetched again")
	}
	// An unrelated historical subscription must not supply the current plan.
	if _, err = db.DB().ExecContext(ctx, `INSERT INTO subscriptions(id,account_id,price_id,plan_id,status,current_period_start,current_period_end) VALUES('sub_old',?,'price_scale','scale','canceled',?,?)`, a.ID, store.Stamp(now), store.Stamp(now.Add(48*time.Hour))); err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET plan_id='scale' WHERE id=?`, a.ID); err != nil {
		t.Fatal(err)
	}
	ent, err = m.Entitlement(ctx, a.ID)
	if err != nil || ent.Plan.ID != "builder" {
		t.Fatalf("mixed subscription entitlement %+v %v", ent, err)
	}
	state = "past_due"
	apply("evt_failed_payment")
	ent, err = m.Entitlement(ctx, a.ID)
	if err != nil || ent.Plan.ID != "builder" {
		t.Fatalf("missing grace %+v %v", ent, err)
	}
	var deadline string
	if err = db.DB().QueryRowContext(ctx, `SELECT grace_until FROM subscriptions WHERE id='sub_test'`).Scan(&deadline); err != nil {
		t.Fatal(err)
	}
	apply("evt_failed_payment_again")
	var repeated string
	if err = db.DB().QueryRowContext(ctx, `SELECT grace_until FROM subscriptions WHERE id='sub_test'`).Scan(&repeated); err != nil || repeated != deadline {
		t.Fatalf("grace extended: %q %q %v", deadline, repeated, err)
	}
	m.Clock = func() time.Time { return now.Add(96 * time.Hour) }
	ent, err = m.Entitlement(ctx, a.ID)
	if err != nil || ent.Plan.ID != "free" {
		t.Fatalf("expired grace %+v %v", ent, err)
	}
	m.Clock = nil
	state = "canceled"
	apply("evt_old_delivered_late")
	ent, err = m.Entitlement(ctx, a.ID)
	if err != nil || ent.Plan.ID != "free" {
		t.Fatalf("late event resurrected %+v %v", ent, err)
	}
}

func TestCheckoutSurvivesRestartAndPreventsSecondPlan(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	a, err := db.CreateAccount(ctx, "checkout@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_test' WHERE id=?`, a.ID); err != nil {
		t.Fatal(err)
	}
	var creates atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/subscriptions":
			_, _ = w.Write([]byte(`{"data":[],"has_more":false}`))
		case r.Method == "POST" && r.URL.Path == "/checkout/sessions":
			if r.Header.Get("Idempotency-Key") == "" {
				t.Error("missing stable idempotency key")
			}
			creates.Add(1)
			_, _ = w.Write([]byte(`{"id":"cs_test","url":"https://checkout.stripe.com/test","status":"open"}`))
		case r.Method == "GET" && r.URL.Path == "/checkout/sessions/cs_test":
			_, _ = w.Write([]byte(`{"id":"cs_test","url":"https://checkout.stripe.com/test","status":"open"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer provider.Close()
	cfg := billing.Config{BaseURL: provider.URL, BuilderPrice: "price_builder", ScalePrice: "price_scale"}
	s := &billing.Service{Store: db, Config: cfg}
	first, err := s.Checkout(ctx, a.ID, "builder")
	if err != nil {
		t.Fatal(err)
	}
	// An open checkout has no subscription yet. Reconciliation must retain
	// its durable attempt so retries cannot create another payment session.
	if _, err = s.ReconcileCustomer(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	s = &billing.Service{Store: db, Config: cfg}
	if _, err = s.Checkout(ctx, a.ID, "scale"); err == nil {
		t.Fatal("second plan checkout allowed")
	}
	again, err := s.Checkout(ctx, a.ID, "builder")
	if err != nil || first != again || creates.Load() != 1 {
		t.Fatalf("retry=%q creates=%d err=%v", again, creates.Load(), err)
	}
}

func TestReconcileCustomerAndCloseBillingAccount(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	a, err := db.CreateAccount(ctx, "reconcile@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_reconcile' WHERE id=?`, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `INSERT INTO checkout_attempts(account_id,id,price_id,created_at) VALUES(?,?,?,?)`, a.ID, "attempt", "price_builder", store.Stamp(time.Now())); err != nil {
		t.Fatal(err)
	}
	state := struct {
		subscription string
		session      string
	}{subscription: "active", session: "open"}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/subscriptions":
			if state.subscription == "" {
				_, _ = w.Write([]byte(`{"data":[],"has_more":false}`))
				return
			}
			_, _ = fmt.Fprintf(w, `{"data":[{"id":"sub_reconcile","customer":"cus_reconcile","status":%q,"items":{"data":[{"price":{"id":"price_builder"},"current_period_start":%d,"current_period_end":%d}]} }],"has_more":false}`, state.subscription, time.Now().Add(-time.Hour).Unix(), time.Now().Add(23*time.Hour).Unix())
		case r.Method == "GET" && r.URL.Path == "/checkout/sessions/cs_reconcile":
			_, _ = fmt.Fprintf(w, `{"id":"cs_reconcile","status":%q}`, state.session)
		case r.Method == "POST" && r.URL.Path == "/checkout/sessions/cs_reconcile/expire":
			state.session = "expired"
			_, _ = w.Write([]byte(`{"id":"cs_reconcile","status":"expired"}`))
		case r.Method == "DELETE" && r.URL.Path == "/subscriptions/sub_reconcile":
			state.subscription = "canceled"
			_, _ = w.Write([]byte(`{"id":"sub_reconcile","customer":"cus_reconcile","status":"canceled"}`))
		default:
			t.Errorf("unexpected provider request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer provider.Close()
	s := &billing.Service{Store: db, Config: billing.Config{BaseURL: provider.URL, BuilderPrice: "price_builder", ScalePrice: "price_scale"}}
	got, err := s.ReconcileCustomer(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Eligible || got.PlanID != "builder" || got.Source != "stripe_subscription" {
		t.Fatalf("unexpected reconciliation result: %+v", got)
	}
	var plan string
	if err = db.DB().QueryRowContext(ctx, `SELECT plan_id FROM accounts WHERE id=?`, a.ID).Scan(&plan); err != nil || plan != "builder" {
		t.Fatalf("plan=%q err=%v", plan, err)
	}
	var attempts int
	if err = db.DB().QueryRowContext(ctx, `SELECT count(*) FROM checkout_attempts WHERE account_id=?`, a.ID).Scan(&attempts); err != nil || attempts != 0 {
		t.Fatalf("stale checkout attempts=%d err=%v", attempts, err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET status='deleting' WHERE id=?`, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `INSERT INTO checkout_attempts(account_id,id,price_id,session_id,created_at) VALUES(?,?,?,?,?)`, a.ID, "attempt-2", "price_builder", "cs_reconcile", store.Stamp(time.Now())); err != nil {
		t.Fatal(err)
	}
	closed, err := s.CloseBillingAccount(ctx, a.ID)
	if err != nil || closed.Status != 1 {
		t.Fatalf("close result=%+v err=%v", closed, err)
	}
	if strings.TrimSpace(state.subscription) != "canceled" || state.session != "expired" {
		t.Fatalf("provider state subscription=%q session=%q", state.subscription, state.session)
	}
}

func TestReconcileRejectsUnknownPriceWithoutGrantingAccess(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	a, err := db.CreateAccount(ctx, "unknown-price@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_unknown' WHERE id=?`, a.ID); err != nil {
		t.Fatal(err)
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"sub_unknown","customer":"cus_unknown","status":"active","items":{"data":[{"price":{"id":"price_other"}}]}}],"has_more":false}`))
	}))
	defer provider.Close()
	s := &billing.Service{Store: db, Config: billing.Config{BaseURL: provider.URL, BuilderPrice: "price_builder", ScalePrice: "price_scale"}}
	if _, err = s.ReconcileCustomer(ctx, a.ID); err == nil || !strings.Contains(err.Error(), "unknown provider price") {
		t.Fatalf("unexpected error: %v", err)
	}
	var plan string
	if err = db.DB().QueryRowContext(ctx, `SELECT plan_id FROM accounts WHERE id=?`, a.ID).Scan(&plan); err != nil || plan != "free" {
		t.Fatalf("unknown price changed plan=%q err=%v", plan, err)
	}
}

// TestReconcileCustomerAnchorsGraceToAuthoritativePeriodNotProcessingTime
// reproduces "missed activation": the account's past_due transition webhook
// was never delivered, and a reconciliation poll days later is the first
// observation. The Subscription and Invoice objects carry no "became
// past_due at" field, so the only authoritative anchor available from a
// snapshot fetch is the subscription's own current period start. Anchoring
// to local processing time instead would let a stale/delayed reconcile
// manufacture a fresh 72h grace window out of thin air.
func TestReconcileCustomerAnchorsGraceToAuthoritativePeriodNotProcessingTime(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	a, err := db.CreateAccount(ctx, "missed-activation@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_missed' WHERE id=?`, a.ID); err != nil {
		t.Fatal(err)
	}
	periodStart := time.Now().UTC().Add(-10 * 24 * time.Hour).Truncate(time.Second)
	periodEnd := periodStart.Add(30 * 24 * time.Hour)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"data":[{"id":"sub_missed","customer":"cus_missed","status":"past_due","items":{"data":[{"price":{"id":"price_builder"},"current_period_start":%d,"current_period_end":%d}]}}],"has_more":false}`, periodStart.Unix(), periodEnd.Unix())
	}))
	defer provider.Close()
	s := &billing.Service{Store: db, Config: billing.Config{BaseURL: provider.URL, BuilderPrice: "price_builder", ScalePrice: "price_scale"}}
	got, err := s.ReconcileCustomer(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := periodStart.Add(72 * time.Hour)
	if !got.GraceUntil.Equal(want) {
		t.Fatalf("grace = %v, want %v (anchored to period start, not processing time)", got.GraceUntil, want)
	}
	if got.GraceUntil.After(time.Now()) {
		t.Fatalf("grace %v should already be past for a period this old; processing-time anchoring would wrongly grant a fresh window", got.GraceUntil)
	}
}

// TestReconcileCustomerRenewalOpensFreshGraceAndRetainsOldWindow reproduces
// two reconciles of the same subscription across a renewal: the account
// fails to pay again in a new billing period after already having a stored
// grace deadline for the old one. Acceptance requires a fresh allowance for
// the new window while the old window's accounting is retained, not
// silently overwritten.
func TestReconcileCustomerRenewalOpensFreshGraceAndRetainsOldWindow(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	a, err := db.CreateAccount(ctx, "renewal@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_renewal' WHERE id=?`, a.ID); err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Truncate(time.Second)
	p1Start, p1End := base.Add(-100*time.Hour), base.Add(-50*time.Hour)
	p2Start, p2End := p1End, base.Add(20*time.Hour)
	period := struct{ start, end time.Time }{p1Start, p1End}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"data":[{"id":"sub_renewal","customer":"cus_renewal","status":"past_due","items":{"data":[{"price":{"id":"price_builder"},"current_period_start":%d,"current_period_end":%d}]}}],"has_more":false}`, period.start.Unix(), period.end.Unix())
	}))
	defer provider.Close()
	s := &billing.Service{Store: db, Config: billing.Config{BaseURL: provider.URL, BuilderPrice: "price_builder", ScalePrice: "price_scale"}}
	got, err := s.ReconcileCustomer(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	grace1 := got.GraceUntil
	if want := p1Start.Add(72 * time.Hour); !grace1.Equal(want) {
		t.Fatalf("first grace = %v, want %v", grace1, want)
	}
	period.start, period.end = p2Start, p2End
	got, err = s.ReconcileCustomer(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	grace2 := got.GraceUntil
	want2 := p2Start.Add(72 * time.Hour)
	if !grace2.Equal(want2) {
		t.Fatalf("renewed grace = %v, want %v", grace2, want2)
	}
	if grace2.Equal(grace1) {
		t.Fatal("renewal did not open a fresh grace window")
	}
	var closed int
	if err = db.DB().QueryRowContext(ctx, `SELECT count(*) FROM audit_log WHERE account_id=? AND action='subscription_window_closed'`, a.ID).Scan(&closed); err != nil || closed != 1 {
		t.Fatalf("old window history entries=%d err=%v", closed, err)
	}
}

// TestWebhookGraceAnchoredToEventCreatedNotProcessingTime reproduces a
// webhook delivered hours after Stripe generated it (retry backoff after an
// outage). Anchoring the grace deadline to local processing time would grant
// a window that starts hours later than the provider actually granted.
func TestWebhookGraceAnchoredToEventCreatedNotProcessingTime(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	a, err := db.CreateAccount(ctx, "delayed-webhook@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_delayed' WHERE id=?`, a.ID); err != nil {
		t.Fatal(err)
	}
	periodStart := time.Now().Add(-time.Hour)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"sub_delayed","customer":"cus_delayed","status":"past_due","items":{"data":[{"price":{"id":"price_builder"},"current_period_start":%d,"current_period_end":%d}]}}`, periodStart.Unix(), periodStart.Add(24*time.Hour).Unix())
	}))
	defer provider.Close()
	s := &billing.Service{Store: db, Config: billing.Config{SecretKey: "test", WebhookSecret: "secret", BuilderPrice: "price_builder", ScalePrice: "price_scale", BaseURL: provider.URL}}
	eventCreated := time.Now().Add(-6 * time.Hour).Truncate(time.Second)
	body, _ := json.Marshal(map[string]any{"id": "evt_delayed", "type": "customer.subscription.updated", "created": eventCreated.Unix(), "data": map[string]any{"object": map[string]any{"id": "sub_delayed", "status": "past_due"}}})
	if err = s.Webhook(ctx, body, signature(body, "secret", time.Now())); err != nil {
		t.Fatal(err)
	}
	var deadline string
	if err = db.DB().QueryRowContext(ctx, `SELECT grace_until FROM subscriptions WHERE id='sub_delayed'`).Scan(&deadline); err != nil {
		t.Fatal(err)
	}
	got, err := time.Parse(time.RFC3339Nano, deadline)
	if err != nil {
		t.Fatal(err)
	}
	want := eventCreated.Add(72 * time.Hour)
	if !got.Equal(want) {
		t.Fatalf("grace = %v, want %v (anchored to event.Created, not processing time)", got, want)
	}
}

// TestWebhookLateFailureAfterPaymentDoesNotReopenGrace reproduces a
// reordered/retried delivery of an old payment-failure event arriving after
// the account already recovered. The webhook must trust only a fresh
// provider fetch, never the event's own claimed status or age.
func TestWebhookLateFailureAfterPaymentDoesNotReopenGrace(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	a, err := db.CreateAccount(ctx, "recovered@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_recovered' WHERE id=?`, a.ID); err != nil {
		t.Fatal(err)
	}
	state := "active"
	now := time.Now()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"sub_recovered","customer":"cus_recovered","status":%q,"items":{"data":[{"price":{"id":"price_builder"},"current_period_start":%d,"current_period_end":%d}]}}`, state, now.Unix(), now.Add(24*time.Hour).Unix())
	}))
	defer provider.Close()
	s := &billing.Service{Store: db, Config: billing.Config{SecretKey: "test", WebhookSecret: "secret", BuilderPrice: "price_builder", ScalePrice: "price_scale", BaseURL: provider.URL}}
	apply := func(id string, created time.Time) {
		body, _ := json.Marshal(map[string]any{"id": id, "type": "customer.subscription.updated", "created": created.Unix(), "data": map[string]any{"object": map[string]any{"id": "sub_recovered", "status": "stale-untrusted"}}})
		if e := s.Webhook(ctx, body, signature(body, "secret", time.Now())); e != nil {
			t.Fatal(e)
		}
	}
	failedAt := now.Add(-2 * time.Hour)
	state = "past_due"
	apply("evt_failed", failedAt)
	recoveredAt := now.Add(-time.Hour)
	state = "active"
	apply("evt_recovered", recoveredAt)
	var grace sql.NullString
	if err = db.DB().QueryRowContext(ctx, `SELECT grace_until FROM subscriptions WHERE id='sub_recovered'`).Scan(&grace); err != nil || grace.Valid {
		t.Fatalf("grace not cleared after payment: %q valid=%v err=%v", grace.String, grace.Valid, err)
	}
	m := &meter.Meter{Store: db}
	ent, err := m.Entitlement(ctx, a.ID)
	if err != nil || ent.Plan.ID != "builder" {
		t.Fatalf("entitlement after recovery %+v %v", ent, err)
	}
	apply("evt_failed_retry", failedAt)
	if err = db.DB().QueryRowContext(ctx, `SELECT grace_until FROM subscriptions WHERE id='sub_recovered'`).Scan(&grace); err != nil || grace.Valid {
		t.Fatalf("late failure event reopened grace: %q valid=%v err=%v", grace.String, grace.Valid, err)
	}
	ent, err = m.Entitlement(ctx, a.ID)
	if err != nil || ent.Plan.ID != "builder" {
		t.Fatalf("entitlement after late failure event %+v %v", ent, err)
	}
}

// TestReconcileRejectsDuplicateNonterminalSubscriptions covers the required
// "duplicate subscription" fixture state: the provider reports two
// nonterminal subscriptions for one customer, which must fail closed rather
// than pick one arbitrarily.
func TestReconcileRejectsDuplicateNonterminalSubscriptions(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	a, err := db.CreateAccount(ctx, "duplicate@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_duplicate' WHERE id=?`, a.ID); err != nil {
		t.Fatal(err)
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		now := time.Now()
		_, _ = fmt.Fprintf(w, `{"data":[{"id":"sub_a","customer":"cus_duplicate","status":"active","items":{"data":[{"price":{"id":"price_builder"},"current_period_start":%d,"current_period_end":%d}]}},{"id":"sub_b","customer":"cus_duplicate","status":"active","items":{"data":[{"price":{"id":"price_scale"},"current_period_start":%d,"current_period_end":%d}]}}],"has_more":false}`, now.Unix(), now.Add(time.Hour).Unix(), now.Unix(), now.Add(time.Hour).Unix())
	}))
	defer provider.Close()
	s := &billing.Service{Store: db, Config: billing.Config{BaseURL: provider.URL, BuilderPrice: "price_builder", ScalePrice: "price_scale"}}
	if _, err = s.ReconcileCustomer(ctx, a.ID); err == nil || !strings.Contains(err.Error(), "duplicate nonterminal subscriptions") {
		t.Fatalf("unexpected error: %v", err)
	}
	var plan string
	if err = db.DB().QueryRowContext(ctx, `SELECT plan_id FROM accounts WHERE id=?`, a.ID).Scan(&plan); err != nil || plan != "free" {
		t.Fatalf("duplicate subscriptions changed plan=%q err=%v", plan, err)
	}
}

// TestReconcileCustomerProviderOutageLeavesLocalStateUntouched covers the
// required "provider outage" fixture state: a 5xx from the provider must
// leave the account's existing entitlement and subscription rows exactly as
// they were, never silently downgraded or upgraded.
func TestReconcileCustomerProviderOutageLeavesLocalStateUntouched(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	a, err := db.CreateAccount(ctx, "outage@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_outage',plan_id='builder' WHERE id=?`, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `INSERT INTO subscriptions(id,account_id,price_id,plan_id,status,current_period_start,current_period_end) VALUES('sub_outage',?,'price_builder','builder','active',?,?)`, a.ID, store.Stamp(time.Now()), store.Stamp(time.Now().Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer provider.Close()
	s := &billing.Service{Store: db, Config: billing.Config{BaseURL: provider.URL, BuilderPrice: "price_builder", ScalePrice: "price_scale"}}
	if _, err = s.ReconcileCustomer(ctx, a.ID); !errors.Is(err, contracts.ErrBillingProviderUnavailable) {
		t.Fatalf("expected provider-unavailable error, got %v", err)
	}
	var plan, status string
	if err = db.DB().QueryRowContext(ctx, `SELECT plan_id FROM accounts WHERE id=?`, a.ID).Scan(&plan); err != nil || plan != "builder" {
		t.Fatalf("outage mutated plan=%q err=%v", plan, err)
	}
	if err = db.DB().QueryRowContext(ctx, `SELECT status FROM subscriptions WHERE id='sub_outage'`).Scan(&status); err != nil || status != "active" {
		t.Fatalf("outage mutated subscription status=%q err=%v", status, err)
	}
}

// TestCloseBillingAccountResumesAfterAlreadyClosedProviderState covers
// "resume after ambiguous provider success and local timeout": a prior call
// reached provider closure (subscription canceled, session expired) but a
// local checkout_attempts row is still present, as if the local commit that
// should have cleared it never landed. A resumed call must certify closed
// again, idempotently, without erroring on already-terminal provider state.
func TestCloseBillingAccountResumesAfterAlreadyClosedProviderState(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	a, err := db.CreateAccount(ctx, "resume-close@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_resume',status='deleting' WHERE id=?`, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `INSERT INTO checkout_attempts(account_id,id,price_id,session_id,created_at) VALUES(?,?,?,?,?)`, a.ID, "attempt-resume", "price_builder", "cs_resume", store.Stamp(time.Now())); err != nil {
		t.Fatal(err)
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == "GET" && r.URL.Path == "/subscriptions":
			_, _ = w.Write([]byte(`{"data":[],"has_more":false}`))
		case r.Method == "GET" && r.URL.Path == "/checkout/sessions/cs_resume":
			_, _ = w.Write([]byte(`{"id":"cs_resume","status":"expired"}`))
		default:
			t.Errorf("unexpected provider request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer provider.Close()
	s := &billing.Service{Store: db, Config: billing.Config{BaseURL: provider.URL, BuilderPrice: "price_builder", ScalePrice: "price_scale"}}
	first, err := s.CloseBillingAccount(ctx, a.ID)
	if err != nil || first.Status != contracts.CloseStatusClosed {
		t.Fatalf("first close=%+v err=%v", first, err)
	}
	second, err := s.CloseBillingAccount(ctx, a.ID)
	if err != nil || second.Status != contracts.CloseStatusClosed {
		t.Fatalf("resumed close=%+v err=%v", second, err)
	}
	var attempts int
	if err = db.DB().QueryRowContext(ctx, `SELECT count(*) FROM checkout_attempts WHERE account_id=?`, a.ID).Scan(&attempts); err != nil || attempts != 0 {
		t.Fatalf("stale checkout attempts=%d err=%v", attempts, err)
	}
}

// TestWebhookUnknownPriceAcknowledgesWithoutGrantingAccess exercises the
// webhook-side counterpart of the reconcile unknown-price rejection: a
// subscription priced outside the two Serenity prices must fail closed
// rather than silently mapping to a plan.
func TestWebhookUnknownPriceAcknowledgesWithoutGrantingAccess(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	a, err := db.CreateAccount(ctx, "webhook-unknown-price@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_webhook_unknown' WHERE id=?`, a.ID); err != nil {
		t.Fatal(err)
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"sub_webhook_unknown","customer":"cus_webhook_unknown","status":"active","items":{"data":[{"price":{"id":"price_other"},"current_period_start":0,"current_period_end":0}]}}`))
	}))
	defer provider.Close()
	s := &billing.Service{Store: db, Config: billing.Config{SecretKey: "test", WebhookSecret: "secret", BuilderPrice: "price_builder", ScalePrice: "price_scale", BaseURL: provider.URL}}
	body, _ := json.Marshal(map[string]any{"id": "evt_unknown_price", "type": "customer.subscription.updated", "created": time.Now().Unix(), "data": map[string]any{"object": map[string]any{"id": "sub_webhook_unknown", "status": "stale-untrusted"}}})
	if err = s.Webhook(ctx, body, signature(body, "secret", time.Now())); err == nil || !strings.Contains(err.Error(), "not a Serenity price") {
		t.Fatalf("unexpected error: %v", err)
	}
	var plan string
	if err = db.DB().QueryRowContext(ctx, `SELECT plan_id FROM accounts WHERE id=?`, a.ID).Scan(&plan); err != nil || plan != "free" {
		t.Fatalf("unknown price changed plan=%q err=%v", plan, err)
	}
}

// TestReconcileIncompleteSubscriptionGrantsNoAccess covers the required
// "initial incomplete/failure" fixture state: a subscription still waiting
// on its first payment attempt (real Stripe semantics: up to 23h before
// incomplete_expired) must not be selected as the entitling subscription and
// must not itself be treated as an error.
func TestReconcileIncompleteSubscriptionGrantsNoAccess(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	a, err := db.CreateAccount(ctx, "incomplete@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_incomplete' WHERE id=?`, a.ID); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"data":[{"id":"sub_incomplete","customer":"cus_incomplete","status":"incomplete","items":{"data":[{"price":{"id":"price_builder"},"current_period_start":%d,"current_period_end":%d}]}}],"has_more":false}`, now.Unix(), now.Add(23*time.Hour).Unix())
	}))
	defer provider.Close()
	s := &billing.Service{Store: db, Config: billing.Config{BaseURL: provider.URL, BuilderPrice: "price_builder", ScalePrice: "price_scale"}}
	got, err := s.ReconcileCustomer(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Eligible || got.PlanID != "" {
		t.Fatalf("incomplete subscription granted access: %+v", got)
	}
	var plan string
	if err = db.DB().QueryRowContext(ctx, `SELECT plan_id FROM accounts WHERE id=?`, a.ID).Scan(&plan); err != nil || plan != "free" {
		t.Fatalf("incomplete subscription changed plan=%q err=%v", plan, err)
	}
}

// TestReconcileScheduledCancellationPreservesAccessUntilPeriodEnd covers the
// cancellation-at-period-end fixture state (not a scheduled price downgrade):
// cancel_at_period_end must be recorded without revoking current access,
// matching the documented Portal behavior above.
func TestReconcileScheduledCancellationPreservesAccessUntilPeriodEnd(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	a, err := db.CreateAccount(ctx, "scheduled-cancel@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_scheduled' WHERE id=?`, a.ID); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"data":[{"id":"sub_scheduled","customer":"cus_scheduled","status":"active","cancel_at_period_end":true,"items":{"data":[{"price":{"id":"price_builder"},"current_period_start":%d,"current_period_end":%d}]}}],"has_more":false}`, now.Unix(), now.Add(20*24*time.Hour).Unix())
	}))
	defer provider.Close()
	s := &billing.Service{Store: db, Config: billing.Config{BaseURL: provider.URL, BuilderPrice: "price_builder", ScalePrice: "price_scale"}}
	got, err := s.ReconcileCustomer(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Eligible || got.PlanID != "builder" {
		t.Fatalf("scheduled cancellation revoked access early: %+v", got)
	}
	var cancelAtPeriodEnd int
	if err = db.DB().QueryRowContext(ctx, `SELECT cancel_at_period_end FROM subscriptions WHERE id='sub_scheduled'`).Scan(&cancelAtPeriodEnd); err != nil || cancelAtPeriodEnd != 1 {
		t.Fatalf("cancel_at_period_end not recorded: %d err=%v", cancelAtPeriodEnd, err)
	}
}

func TestWebhookPriceChangesCancellationAndResubscription(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	a, err := db.CreateAccount(ctx, "lifecycle@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_cycle' WHERE id=?`, a.ID); err != nil {
		t.Fatal(err)
	}
	var payload atomic.Value
	now := time.Now().UTC()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload.Load().(string)))
	}))
	t.Cleanup(provider.Close)
	svc := &billing.Service{Store: db, Config: billing.Config{BaseURL: provider.URL, WebhookSecret: "secret", BuilderPrice: "price_builder", ScalePrice: "price_scale"}}
	for i, step := range []struct{ name, id, status, price, plan string }{
		{"activate", "sub_cycle", "active", "price_builder", "builder"},
		{"upgrade_same_subscription", "sub_cycle", "active", "price_scale", "scale"},
		{"downgrade_applied_by_provider", "sub_cycle", "active", "price_builder", "builder"},
		{"cancel", "sub_cycle", "canceled", "price_builder", "free"},
		{"resubscribe_new_subscription", "sub_again", "active", "price_scale", "scale"},
	} {
		t.Run(step.name, func(t *testing.T) {
			payload.Store(fmt.Sprintf(`{"id":%q,"customer":"cus_cycle","status":%q,"items":{"data":[{"price":{"id":%q},"current_period_start":%d,"current_period_end":%d}]}}`, step.id, step.status, step.price, now.Unix(), now.Add(30*24*time.Hour).Unix()))
			body, err := json.Marshal(map[string]any{"id": fmt.Sprintf("evt_cycle_%d", i), "type": "customer.subscription.updated", "created": now.Unix(), "data": map[string]any{"object": map[string]any{"id": step.id}}})
			if err != nil {
				t.Fatal(err)
			}
			if err := svc.Webhook(ctx, body, signature(body, "secret", now)); err != nil {
				t.Fatal(err)
			}
			ent, err := (&meter.Meter{Store: db}).Entitlement(ctx, a.ID)
			if err != nil || ent.Plan.ID != step.plan {
				t.Fatalf("plan=%s want=%s err=%v", ent.Plan.ID, step.plan, err)
			}
			var count int
			if err := db.DB().QueryRowContext(ctx, `SELECT count(*) FROM subscriptions WHERE account_id=? AND status='active'`, a.ID).Scan(&count); err != nil {
				t.Fatal(err)
			}
			want := 1
			if step.status == "canceled" {
				want = 0
			}
			if count != want {
				t.Fatalf("active subscriptions=%d want=%d", count, want)
			}
		})
	}
}

func TestWebhookProviderOutageLeavesEventRetryable(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	a, err := db.CreateAccount(ctx, "retry@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_retry' WHERE id=?`, a.ID); err != nil {
		t.Fatal(err)
	}
	var unavailable atomic.Bool
	unavailable.Store(true)
	now := time.Now()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if unavailable.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"sub_retry","customer":"cus_retry","status":"active","items":{"data":[{"price":{"id":"price_builder"},"current_period_start":%d,"current_period_end":%d}]}}`, now.Unix(), now.Add(24*time.Hour).Unix())
	}))
	t.Cleanup(provider.Close)
	svc := &billing.Service{Store: db, Config: billing.Config{BaseURL: provider.URL, WebhookSecret: "secret", BuilderPrice: "price_builder"}}
	body, err := json.Marshal(map[string]any{"id": "evt_retry", "type": "customer.subscription.updated", "created": now.Unix(), "data": map[string]any{"object": map[string]any{"id": "sub_retry"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.Webhook(ctx, body, signature(body, "secret", now)); err == nil {
		t.Fatal("provider outage acknowledged as success")
	}
	var processed sql.NullString
	if err = db.DB().QueryRowContext(ctx, `SELECT processed_at FROM stripe_events WHERE id='evt_retry'`).Scan(&processed); err != nil || processed.Valid {
		t.Fatalf("processed=%v err=%v", processed, err)
	}
	ent, err := (&meter.Meter{Store: db}).Entitlement(ctx, a.ID)
	if err != nil || ent.Plan.ID != "free" {
		t.Fatalf("outage granted access: %+v %v", ent, err)
	}
	unavailable.Store(false)
	if err = svc.Webhook(ctx, body, signature(body, "secret", now)); err != nil {
		t.Fatal(err)
	}
	ent, err = (&meter.Meter{Store: db}).Entitlement(ctx, a.ID)
	if err != nil || ent.Plan.ID != "builder" {
		t.Fatalf("retry did not activate: %+v %v", ent, err)
	}
	if err = db.DB().QueryRowContext(ctx, `SELECT processed_at FROM stripe_events WHERE id='evt_retry'`).Scan(&processed); err != nil || !processed.Valid {
		t.Fatalf("successful retry not recorded: %v %v", processed, err)
	}
}

// Fail the actual local transaction after the provider has accepted cancellation.
// Retry uses a new Service instance to avoid relying on process-local state.
func TestCloseBillingRetriesAfterProviderSuccessAndLocalCommitFailure(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	a, err := db.CreateAccount(ctx, "commit-failure@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET status='deleting',stripe_customer_id='cus_commit',plan_id='builder' WHERE id=?`, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `INSERT INTO subscriptions(id,account_id,price_id,plan_id,status) VALUES('sub_commit',?,'price_builder','builder','active')`, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `CREATE TRIGGER fail_closure BEFORE UPDATE ON subscriptions BEGIN SELECT RAISE(ABORT,'injected local closure failure'); END`); err != nil {
		t.Fatal(err)
	}
	var canceled atomic.Bool
	var cancellations atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/subscriptions/sub_commit":
			cancellations.Add(1)
			canceled.Store(true)
			_, _ = w.Write([]byte(`{"id":"sub_commit","status":"canceled"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/subscriptions":
			status := "active"
			if canceled.Load() {
				status = "canceled"
			}
			_, _ = fmt.Fprintf(w, `{"data":[{"id":"sub_commit","customer":"cus_commit","status":%q,"items":{"data":[{"price":{"id":"price_builder"}}]}}],"has_more":false}`, status)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(provider.Close)
	config := billing.Config{BaseURL: provider.URL, BuilderPrice: "price_builder"}
	got, err := (&billing.Service{Store: db, Config: config}).CloseBillingAccount(ctx, a.ID)
	if err == nil || got.Status != contracts.CloseStatusPending || !canceled.Load() {
		t.Fatalf("closure=%+v err=%v provider canceled=%v", got, err, canceled.Load())
	}
	var status, plan string
	if err = db.DB().QueryRowContext(ctx, `SELECT status FROM subscriptions WHERE id='sub_commit'`).Scan(&status); err != nil || status != "active" {
		t.Fatalf("local transaction did not roll back: %q %v", status, err)
	}
	if err = db.DB().QueryRowContext(ctx, `SELECT status,plan_id FROM accounts WHERE id=?`, a.ID).Scan(&status, &plan); err != nil || status != "deleting" || plan != "builder" {
		t.Fatalf("account state=%s/%s err=%v", status, plan, err)
	}
	if _, err = db.DB().ExecContext(ctx, `DROP TRIGGER fail_closure`); err != nil {
		t.Fatal(err)
	}
	got, err = (&billing.Service{Store: db, Config: config}).CloseBillingAccount(ctx, a.ID)
	if err != nil || got.Status != contracts.CloseStatusClosed {
		t.Fatalf("retry=%+v err=%v", got, err)
	}
	if cancellations.Load() != 1 {
		t.Fatalf("canceled %d times", cancellations.Load())
	}
	if err = db.DB().QueryRowContext(ctx, `SELECT status FROM subscriptions WHERE id='sub_commit'`).Scan(&status); err != nil || status != "canceled" {
		t.Fatalf("local closure missing: %q %v", status, err)
	}
	if err = db.DB().QueryRowContext(ctx, `SELECT status,plan_id FROM accounts WHERE id=?`, a.ID).Scan(&status, &plan); err != nil || status != "deleting" || plan != "free" {
		t.Fatalf("closed account=%s/%s err=%v", status, plan, err)
	}
}

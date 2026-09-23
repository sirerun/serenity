package billing_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
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
		body, _ := json.Marshal(map[string]any{"id": id, "type": "customer.subscription.updated", "data": map[string]any{"object": map[string]any{"id": "sub_test", "status": "stale-untrusted"}}})
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

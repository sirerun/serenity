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

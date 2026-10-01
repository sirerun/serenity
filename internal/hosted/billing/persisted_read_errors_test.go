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
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/store"
)

func persistedReadFixture(t *testing.T) (context.Context, *store.Store, string, *httptest.Server) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	account, err := db.CreateAccount(ctx, "persisted-read@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_read' WHERE id=?`, account.ID); err != nil {
		t.Fatal(err)
	}
	oldStart := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Second)
	if _, err = db.DB().ExecContext(ctx, `INSERT INTO subscriptions(id,account_id,price_id,plan_id,status,current_period_start,current_period_end,grace_until,grace_invoice_id) VALUES('sub_read',?,'price_scale','scale','active',?,?,?,?)`, account.ID, store.Stamp(oldStart), store.Stamp(oldStart.Add(30*24*time.Hour)), store.Stamp(oldStart.Add(72*time.Hour)), "in_old"); err != nil {
		t.Fatal(err)
	}
	periodStart := oldStart.Add(30 * 24 * time.Hour)
	periodEnd := periodStart.Add(30 * 24 * time.Hour)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		sub := fmt.Sprintf(`{"id":"sub_read","customer":"cus_read","status":"active","latest_invoice":"in_current","items":{"data":[{"price":{"id":"price_builder"},"current_period_start":%d,"current_period_end":%d}]}}`, periodStart.Unix(), periodEnd.Unix())
		if r.URL.Path == "/subscriptions" {
			_, _ = fmt.Fprintf(w, `{"data":[%s],"has_more":false}`, sub)
			return
		}
		if r.URL.Path == "/subscriptions/sub_read" {
			_, _ = fmt.Fprint(w, sub)
			return
		}
		http.NotFound(w, r)
	}))
	return ctx, db, account.ID, provider
}

func failPriorSubscriptionRead(err error) func(context.Context, *sql.Tx, string, string) (priorSubscription, error) {
	return func(context.Context, *sql.Tx, string, string) (priorSubscription, error) {
		return priorSubscription{}, err
	}
}

func TestReconcilePriorSubscriptionReadFailureRollsBackRenewal(t *testing.T) {
	ctx, db, accountID, provider := persistedReadFixture(t)
	defer provider.Close()
	readErr := errors.New("injected SQLite statement read failure")
	s := &Service{Store: db, Config: Config{BaseURL: provider.URL, BuilderPrice: "price_builder", ScalePrice: "price_scale"}, priorSubscriptionReader: failPriorSubscriptionRead(readErr)}
	_, err := s.ReconcileCustomer(ctx, accountID)
	var persisted *persistedBillingReadError
	if !errors.As(err, &persisted) || !errors.Is(err, readErr) {
		t.Fatalf("reconcile error = %v, want wrapped persisted read failure", err)
	}
	var plan, start, grace string
	if err = db.DB().QueryRowContext(ctx, `SELECT plan_id FROM accounts WHERE id=?`, accountID).Scan(&plan); err != nil || plan != "free" {
		t.Fatalf("account changed after failed read: plan=%q err=%v", plan, err)
	}
	if err = db.DB().QueryRowContext(ctx, `SELECT plan_id,current_period_start,grace_until FROM subscriptions WHERE id='sub_read'`).Scan(&plan, &start, &grace); err != nil || plan != "scale" || start == "" || grace == "" {
		t.Fatalf("subscription changed after failed read: plan=%q start=%q grace=%q err=%v", plan, start, grace, err)
	}
	var closed, reconciled int
	if err = db.DB().QueryRowContext(ctx, `SELECT count(*) FROM audit_log WHERE action='subscription_window_closed'`).Scan(&closed); err != nil || closed != 0 {
		t.Fatalf("closure audit after failed read: count=%d err=%v", closed, err)
	}
	if err = db.DB().QueryRowContext(ctx, `SELECT count(*) FROM audit_log WHERE action='billing_reconciled'`).Scan(&reconciled); err != nil || reconciled != 0 {
		t.Fatalf("reconcile audit after failed read: count=%d err=%v", reconciled, err)
	}
}

func TestWebhookPriorSubscriptionReadFailureLeavesEventRetryable(t *testing.T) {
	ctx, db, accountID, provider := persistedReadFixture(t)
	defer provider.Close()
	readErr := errors.New("injected SQLite statement read failure")
	s := &Service{Store: db, Config: Config{BaseURL: provider.URL, BuilderPrice: "price_builder", ScalePrice: "price_scale", WebhookSecret: "test-secret"}, priorSubscriptionReader: failPriorSubscriptionRead(readErr)}
	body, err := json.Marshal(map[string]any{"id": "evt_read", "type": "customer.subscription.updated", "created": time.Now().Unix(), "data": map[string]any{"object": map[string]any{"id": "sub_read"}}})
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Now().Unix()
	mac := hmac.New(sha256.New, []byte("test-secret"))
	_, _ = fmt.Fprintf(mac, "%d.", ts)
	_, _ = mac.Write(body)
	signature := fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
	if err = s.Webhook(ctx, body, signature); !errors.Is(err, readErr) {
		t.Fatalf("webhook error = %v, want wrapped persisted read failure", err)
	}
	var plan, start, grace string
	if err = db.DB().QueryRowContext(ctx, `SELECT plan_id FROM subscriptions WHERE id='sub_read'`).Scan(&plan); err != nil || plan != "scale" {
		t.Fatalf("subscription plan changed after failed read: plan=%q err=%v", plan, err)
	}
	if err = db.DB().QueryRowContext(ctx, `SELECT current_period_start,grace_until FROM subscriptions WHERE id='sub_read'`).Scan(&start, &grace); err != nil || start == "" || grace == "" {
		t.Fatalf("subscription window changed after failed read: start=%q grace=%q err=%v", start, grace, err)
	}
	var processed, receipts int
	if err = db.DB().QueryRowContext(ctx, `SELECT count(*) FROM stripe_events WHERE id='evt_read' AND processed_at IS NOT NULL`).Scan(&processed); err != nil || processed != 0 {
		t.Fatalf("failed webhook was marked processed: processed rows=%d err=%v", processed, err)
	}
	if err = db.DB().QueryRowContext(ctx, `SELECT count(*) FROM stripe_events WHERE id='evt_read'`).Scan(&receipts); err != nil || receipts != 1 {
		t.Fatalf("webhook receipt missing after failure: rows=%d err=%v", receipts, err)
	}
	var closed int
	if err = db.DB().QueryRowContext(ctx, `SELECT count(*) FROM audit_log WHERE action='subscription_window_closed' AND account_id=?`, accountID).Scan(&closed); err != nil || closed != 0 {
		t.Fatalf("closure audit after failed read: count=%d err=%v", closed, err)
	}
}

func TestReadPriorSubscriptionAllowsOnlyMissingRow(t *testing.T) {
	ctx, db, accountID, provider := persistedReadFixture(t)
	defer provider.Close()
	s := &Service{Store: db}
	var got priorSubscription
	err := db.Transaction(ctx, func(tx *sql.Tx) error {
		var readErr error
		got, readErr = s.readPriorSubscription(ctx, tx, "not_yet_seen", accountID)
		return readErr
	})
	if err != nil || got != (priorSubscription{}) {
		t.Fatalf("missing prior row = %+v, %v; want empty state and success", got, err)
	}
	readErr := errors.New("injected read failure")
	s.priorSubscriptionReader = failPriorSubscriptionRead(readErr)
	err = db.Transaction(ctx, func(tx *sql.Tx) error {
		_, readErr := s.readPriorSubscription(ctx, tx, "sub_read", accountID)
		return readErr
	})
	var persisted *persistedBillingReadError
	if !errors.As(err, &persisted) || !errors.Is(err, readErr) {
		t.Fatalf("failed prior-row read = %v, want typed wrapped error", err)
	}
}

func TestReconcileReturnsExpiredGraceDeadlineFromTransaction(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	account, err := db.CreateAccount(ctx, "expired-grace@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_expired' WHERE id=?`, account.ID); err != nil {
		t.Fatal(err)
	}
	failureAt := time.Now().UTC().Add(-96 * time.Hour).Truncate(time.Second)
	if _, err = db.DB().ExecContext(ctx, `INSERT INTO billing_failures(account_id,subscription_id,invoice_id,event_id,first_failed_at) VALUES(?,?,?,?,?)`, account.ID, "sub_expired", "in_expired", "evt_expired", store.Stamp(failureAt)); err != nil {
		t.Fatal(err)
	}
	periodStart := failureAt.Add(-24 * time.Hour)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"data":[{"id":"sub_expired","customer":"cus_expired","status":"past_due","latest_invoice":"in_expired","items":{"data":[{"price":{"id":"price_builder"},"current_period_start":%d,"current_period_end":%d}]}}],"has_more":false}`, periodStart.Unix(), periodStart.Add(30*24*time.Hour).Unix())
	}))
	defer provider.Close()
	s := &Service{Store: db, Config: Config{BaseURL: provider.URL, BuilderPrice: "price_builder", ScalePrice: "price_scale"}}
	result, err := s.ReconcileCustomer(ctx, account.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := failureAt.Add(72 * time.Hour)
	if !result.Eligible || !result.GraceUntil.Equal(want) || !result.GraceUntil.Before(time.Now()) {
		t.Fatalf("reconcile result=%+v, want exact expired grace deadline %v", result, want)
	}
}

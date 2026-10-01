package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/billing"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func newReconcileService(t *testing.T, provider http.Handler) (*Service, *store.Store, string, func()) {
	return newReconcileServiceWithStatus(t, provider, "active")
}

func newReconcileServiceWithStatus(t *testing.T, provider http.Handler, status string) (*Service, *store.Store, string, func()) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	account, err := db.CreateAccount(context.Background(), "reconcile-worker@example.test")
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if _, err = db.DB().Exec(`UPDATE accounts SET stripe_customer_id='cus_worker',status=? WHERE id=?`, status, account.ID); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	server := httptest.NewServer(provider)
	cfg := Config{
		DataDir: t.TempDir(), MaxOpen: 2, MaxInFlight: 4, PublicOrigin: "http://127.0.0.1",
		billingConfig: &billing.Config{BaseURL: server.URL, BuilderPrice: "price_builder", ScalePrice: "price_scale"},
	}
	svc, err := Assemble(cfg, true, db, nil, deletionEmbedding{})
	if err != nil {
		server.Close()
		_ = db.Close()
		t.Fatal(err)
	}
	return svc, db, account.ID, func() {
		_ = svc.Close()
		server.Close()
	}
}

func emptySubscriptionProvider(t *testing.T, onRequest func()) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/subscriptions" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		if onRequest != nil {
			onRequest()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}, "has_more": false})
	})
}

func waitFor(t *testing.T, description string, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

func TestBillingReconcileWorkerUsesConfiguredProviderAndPreservesFreeze(t *testing.T) {
	var requests atomic.Int32
	_, db, accountID, cleanup := newReconcileService(t, emptySubscriptionProvider(t, func() { requests.Add(1) }))
	defer cleanup()
	waitFor(t, "billing reconciliation audit", func() bool {
		var count int
		_ = db.DB().QueryRow(`SELECT count(*) FROM audit_log WHERE account_id=? AND action='billing_reconciled'`, accountID).Scan(&count)
		return count > 0
	})
	if requests.Load() == 0 {
		t.Fatal("configured worker did not call the provider")
	}
	var status, plan string
	if err := db.DB().QueryRow(`SELECT status,plan_id FROM accounts WHERE id=?`, accountID).Scan(&status, &plan); err != nil {
		t.Fatal(err)
	}
	if status != "active" || plan != "free" {
		t.Fatalf("account status/plan = %q/%q", status, plan)
	}
}

func TestBillingReconcileWorkerKeepsRestorePendingRestricted(t *testing.T) {
	var requests atomic.Int32
	_, db, accountID, cleanup := newReconcileServiceWithStatus(t,
		emptySubscriptionProvider(t, func() { requests.Add(1) }), "restore_pending")
	defer cleanup()
	waitFor(t, "restore-pending reconciliation bookkeeping", func() bool {
		var count int
		_ = db.DB().QueryRow(`SELECT count(*) FROM audit_log WHERE account_id=? AND action='billing_reconciled'`, accountID).Scan(&count)
		return count > 0
	})
	if requests.Load() == 0 {
		t.Fatal("restore-pending account was not reconciled")
	}
	var status, plan string
	if err := db.DB().QueryRow(`SELECT status,plan_id FROM accounts WHERE id=?`, accountID).Scan(&status, &plan); err != nil {
		t.Fatal(err)
	}
	if status != "restore_pending" || plan != "free" {
		t.Fatalf("restore-pending status/plan = %q/%q", status, plan)
	}
}

func TestBillingReconcileWorkerDoesNotCallProviderWhenDisabled(t *testing.T) {
	var requests atomic.Int32
	provider := httptest.NewServer(emptySubscriptionProvider(t, func() { requests.Add(1) }))
	db, err := store.Open(filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	account, err := db.CreateAccount(context.Background(), "billing-disabled@example.test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().Exec(`UPDATE accounts SET stripe_customer_id='cus_worker' WHERE id=?`, account.ID); err != nil {
		t.Fatal(err)
	}
	svc, err := Assemble(Config{DataDir: t.TempDir(), MaxOpen: 2, MaxInFlight: 4, PublicOrigin: "http://127.0.0.1"}, true, db, nil, deletionEmbedding{})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if requests.Load() != 0 {
		t.Fatal("billing-disabled assembly called the provider")
	}
	if err = svc.Close(); err != nil {
		t.Fatal(err)
	}
	provider.Close()
}

func TestBillingReconcileWorkerRepairsStaleActiveReadAfterFreeze(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/subscriptions" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		close(entered)
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{
			map[string]any{
				"id": "sub_worker", "customer": "cus_worker", "status": "active",
				"items": map[string]any{"data": []any{map[string]any{
					"price":                map[string]any{"id": "price_builder"},
					"current_period_start": time.Now().Add(-time.Hour).Unix(),
					"current_period_end":   time.Now().Add(24 * time.Hour).Unix(),
				}}},
			},
		}, "has_more": false})
	})
	_, db, accountID, cleanup := newReconcileService(t, provider)
	defer cleanup()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not enter provider request")
	}
	if _, err := db.DB().Exec(`UPDATE accounts SET status='restore_pending',plan_id='free',plan_version=1 WHERE id=?`, accountID); err != nil {
		close(release)
		t.Fatal(err)
	}
	close(release)
	waitFor(t, "post-reconcile frozen plan repair", func() bool {
		var status, plan string
		var subscriptions, reconciliations int
		if err := db.DB().QueryRow(`SELECT a.status,a.plan_id,
			(SELECT count(*) FROM subscriptions WHERE account_id=a.id),
			(SELECT count(*) FROM audit_log WHERE account_id=a.id AND action='billing_reconciled')
			FROM accounts a WHERE a.id=?`, accountID).Scan(&status, &plan, &subscriptions, &reconciliations); err != nil {
			return false
		}
		return status == "restore_pending" && plan == "free" && subscriptions == 1 && reconciliations > 0
	})
	var plan string
	if err := db.DB().QueryRow(`SELECT plan_id FROM accounts WHERE id=?`, accountID).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	if plan != "free" {
		t.Fatalf("frozen account plan = %q", plan)
	}
}

func runOldCheckoutExpiry(t *testing.T, expiryResponse string) (*store.Store, string, <-chan struct{}, func()) {
	t.Helper()
	listEntered := make(chan struct{})
	releaseList := make(chan struct{})
	expiryPosted := make(chan struct{})
	var posts atomic.Int32
	var enteredOnce, postOnce atomic.Bool
	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /subscriptions":
			if enteredOnce.CompareAndSwap(false, true) {
				close(listEntered)
			}
			select {
			case <-releaseList:
			case <-r.Context().Done():
				return
			}
			_, _ = w.Write([]byte(`{"data":[],"has_more":false}`))
		case "GET /checkout/sessions/cs_old":
			_, _ = w.Write([]byte(`{"id":"cs_old","status":"open"}`))
		case "POST /checkout/sessions/cs_old/expire":
			posts.Add(1)
			if got := r.Header.Get("Idempotency-Key"); got != "serenity-expire-cs_old" {
				t.Errorf("expiry idempotency key=%q", got)
			}
			_, _ = w.Write([]byte(expiryResponse))
			if postOnce.CompareAndSwap(false, true) {
				close(expiryPosted)
			}
		default:
			http.Error(w, "unexpected provider request", http.StatusNotFound)
		}
	})
	svc, db, accountID, cleanup := newReconcileService(t, provider)
	select {
	case <-listEntered:
	case <-time.After(5 * time.Second):
		cleanup()
		t.Fatal("worker did not enter subscription request")
	}
	_, err := db.DB().Exec(`INSERT INTO checkout_attempts(account_id,id,price_id,session_id,created_at) VALUES(?,?,?,?,?)`, accountID, store.ID(), "price_builder", "cs_old", store.Stamp(time.Now().Add(-24*time.Hour)))
	if err != nil {
		close(releaseList)
		cleanup()
		t.Fatal(err)
	}
	close(releaseList)
	return db, accountID, expiryPosted, func() {
		_ = svc.Close()
		cleanup()
		if got := posts.Load(); got > 1 {
			t.Errorf("expiry POST count=%d, want at most one", got)
		}
	}
}

func TestBillingReconcileWorkerExpiresOnlyConfirmedOldOpenCheckout(t *testing.T) {
	db, accountID, posted, cleanup := runOldCheckoutExpiry(t, `{"id":"cs_old","status":"expired"}`)
	defer cleanup()
	select {
	case <-posted:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not POST expiration for old open checkout")
	}
	waitFor(t, "confirmed old checkout reconciliation", func() bool {
		var count int
		_ = db.DB().QueryRow(`SELECT count(*) FROM audit_log WHERE account_id=? AND action='checkout_attempt_reconciled' AND detail='expired_open_session'`, accountID).Scan(&count)
		return count == 1
	})
	var attempts int
	if err := db.DB().QueryRow(`SELECT count(*) FROM checkout_attempts WHERE account_id=?`, accountID).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 0 {
		t.Fatalf("confirmed checkout attempt rows=%d, want 0", attempts)
	}
}

func TestBillingReconcileWorkerRetainsAmbiguousOldCheckout(t *testing.T) {
	db, accountID, posted, cleanup := runOldCheckoutExpiry(t, `{"id":"cs_other","status":"expired"}`)
	defer cleanup()
	select {
	case <-posted:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not attempt old checkout expiration")
	}
	// Let the provider response return through the worker before checking that
	// an unconfirmed expiration leaves its durable attempt available for retry.
	time.Sleep(100 * time.Millisecond)
	var attempts, reconciled int
	if err := db.DB().QueryRow(`SELECT count(*) FROM checkout_attempts WHERE account_id=?`, accountID).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if err := db.DB().QueryRow(`SELECT count(*) FROM audit_log WHERE account_id=? AND action='checkout_attempt_reconciled'`, accountID).Scan(&reconciled); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || reconciled != 0 {
		t.Fatalf("ambiguous expiration attempts=%d reconciled=%d, want retained and unaudited", attempts, reconciled)
	}
}

func TestBillingReconcileWorkerCloseCancelsAndJoinsProviderCall(t *testing.T) {
	entered := make(chan struct{})
	providerCanceled := make(chan struct{})
	provider := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
		close(providerCanceled)
	})
	svc, _, _, cleanup := newReconcileService(t, provider)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		cleanup()
		t.Fatal("worker did not enter provider request")
	}
	closed := make(chan error, 1)
	go func() { closed <- svc.Close() }()
	select {
	case <-providerCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("Service.Close did not cancel provider request")
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatalf("Service.Close: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Service.Close did not join worker")
	}
	if err := svc.Close(); err != nil {
		t.Fatalf("repeated Service.Close: %v", err)
	}
	cleanup()
}

func TestBillingRetriesAreBoundedAndExponentiallyDelayed(t *testing.T) {
	var retries billingRetries
	now := time.Now()
	retries.failed("a", now, billingFailureAmbiguous)
	if entry := retries.byAccount["a"]; !entry.next.Equal(now.Add(billingAmbiguousRetryBase)) {
		t.Fatalf("first ambiguous retry=%s, want %s", entry.next.Sub(now), billingAmbiguousRetryBase)
	}
	for n := 0; n < 12; n++ {
		retries.failed("a", now, billingFailureAmbiguous)
	}
	if got := retries.byAccount["a"].next.Sub(now); got != billingAmbiguousRetryMax {
		t.Fatalf("ambiguous retry cap=%s, want %s", got, billingAmbiguousRetryMax)
	}
	for n := 0; n < billingRetryCapacity+1; n++ {
		retries.failed(string(rune(n+1000)), now.Add(time.Duration(n)*time.Nanosecond), billingFailureUnavailable)
	}
	if len(retries.byAccount) > billingRetryCapacity {
		t.Fatalf("retry map size=%d exceeds bound %d", len(retries.byAccount), billingRetryCapacity)
	}
}

func TestBillingRetryClassChangeResetsAttemptExponent(t *testing.T) {
	var retries billingRetries
	now := time.Now()
	for n := 0; n < 5; n++ {
		retries.failed("a", now, billingFailureAmbiguous)
	}
	retries.failed("a", now, billingFailureUnavailable)
	entry := retries.byAccount["a"]
	if entry.class != billingFailureUnavailable || entry.attempt != 1 || !entry.next.Equal(now.Add(billingTransientRetryBase)) {
		t.Fatalf("class transition retained old exponent: class=%s attempt=%d delay=%s", entry.class, entry.attempt, entry.next.Sub(now))
	}
	if got := billingRetryDelay(billingFailureUnavailable, 2); got != 2*time.Minute {
		t.Fatalf("transient second retry=%s, want 2m", got)
	}
	if got := billingRetryDelay(billingFailureUnavailable, 20); got != billingTransientRetryMax {
		t.Fatalf("transient retry cap=%s, want %s", got, billingTransientRetryMax)
	}
	if got := billingRetryDelay(billingFailureAmbiguous, 2); got != 30*time.Minute {
		t.Fatalf("ambiguous second retry=%s, want 30m", got)
	}
}

func TestBillingPageErrorRetainsLastSuccessfulCursor(t *testing.T) {
	var retries billingRetries
	after := ""
	pageReads := 0
	readPage := func(_ context.Context, gotAfter string) ([]string, error) {
		pageReads++
		switch pageReads {
		case 1:
			if gotAfter != "" {
				t.Fatalf("initial cursor=%q", gotAfter)
			}
			ids := make([]string, billingReconcilePageSize)
			for i := range ids {
				ids[i] = fmt.Sprintf("account-%02d", i)
			}
			return ids, nil
		case 2:
			if gotAfter != "account-19" {
				t.Fatalf("page retry cursor=%q, want account-19", gotAfter)
			}
			return nil, errors.New("database unavailable")
		case 3:
			if gotAfter != "account-19" {
				t.Fatalf("cursor after page error=%q, want account-19", gotAfter)
			}
			return []string{"account-20"}, nil
		default:
			t.Fatalf("unexpected page read %d", pageReads)
			return nil, nil
		}
	}
	reconcile := func(context.Context, string) error { return nil }
	complete, err := runBillingPage(context.Background(), &retries, &after, readPage, reconcile)
	if err != nil || complete || after != "account-19" {
		t.Fatalf("first page complete=%t cursor=%q err=%v", complete, after, err)
	}
	complete, err = runBillingPage(context.Background(), &retries, &after, readPage, reconcile)
	if err == nil || complete || after != "account-19" {
		t.Fatalf("failed page complete=%t cursor=%q err=%v", complete, after, err)
	}
	complete, err = runBillingPage(context.Background(), &retries, &after, readPage, reconcile)
	if err != nil || !complete || after != "account-20" {
		t.Fatalf("resumed page complete=%t cursor=%q err=%v", complete, after, err)
	}
}

func TestBillingWorkerFailureLogOmitsAccountAndRawError(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&output, nil)))
	defer slog.SetDefault(previous)
	var retries billingRetries
	after := ""
	readPage := func(context.Context, string) ([]string, error) { return []string{"private-account-id"}, nil }
	reconcile := func(context.Context, string) error {
		return fmt.Errorf("provider response for cus_private: %w", contracts.ErrBillingProviderUnavailable)
	}
	if _, err := runBillingPage(context.Background(), &retries, &after, readPage, reconcile); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, string(billingFailureUnavailable)) || strings.Contains(text, "private-account-id") || strings.Contains(text, "cus_private") {
		t.Fatalf("unexpected reconciliation log content: %q", text)
	}
}

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/billing"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func newReconcileService(t *testing.T, provider http.Handler) (*Service, *store.Store, string, func()) {
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
	if _, err = db.DB().Exec(`UPDATE accounts SET stripe_customer_id='cus_worker' WHERE id=?`, account.ID); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	server := httptest.NewServer(provider)
	cfg := Config{
		DataDir: t.TempDir(), MaxOpen: 2, MaxInFlight: 4,
		billingConfig: &billing.Config{BaseURL: server.URL, BuilderPrice: "price_builder", ScalePrice: "price_scale"},
	}
	svc, err := Assemble(cfg, true, db, nil, nil)
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
	svc, err := Assemble(Config{DataDir: t.TempDir(), MaxOpen: 2, MaxInFlight: 4}, true, db, nil, nil)
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
		if err := db.DB().QueryRow(`SELECT status,plan_id FROM accounts WHERE id=?`, accountID).Scan(&status, &plan); err != nil {
			return false
		}
		return status == "restore_pending" && plan == "free"
	})
	var plan string
	if err := db.DB().QueryRow(`SELECT plan_id FROM accounts WHERE id=?`, accountID).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	if plan != "free" {
		t.Fatalf("frozen account plan = %q", plan)
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
	retries.failed("a", now)
	if entry := retries.byAccount["a"]; !entry.next.Equal(now.Add(time.Minute)) {
		t.Fatalf("first retry=%s, want one minute", entry.next.Sub(now))
	}
	for n := 0; n < 12; n++ {
		retries.failed("a", now)
	}
	if got := retries.byAccount["a"].next.Sub(now); got != billingRetryMax {
		t.Fatalf("retry cap=%s, want %s", got, billingRetryMax)
	}
	for n := 0; n < billingRetryCapacity+1; n++ {
		retries.failed(string(rune(n+1000)), now.Add(time.Duration(n)*time.Nanosecond))
	}
	if len(retries.byAccount) > billingRetryCapacity {
		t.Fatalf("retry map size=%d exceeds bound %d", len(retries.byAccount), billingRetryCapacity)
	}
}

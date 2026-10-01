package billing_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/billing"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func createBillingAccount(t *testing.T, db *store.Store, email, customer string) string {
	t.Helper()
	account, err := db.CreateAccount(context.Background(), email)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(context.Background(), `UPDATE accounts SET stripe_customer_id=? WHERE id=?`, customer, account.ID); err != nil {
		t.Fatal(err)
	}
	return account.ID
}

func openBillingStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}

func TestAccountLocksAllowUnrelatedReconciliationDuringProviderWait(t *testing.T) {
	db := openBillingStore(t)
	accountA := createBillingAccount(t, db, "a@example.test", "cus_a")
	accountB := createBillingAccount(t, db, "b@example.test", "cus_b")
	enteredA := make(chan struct{})
	releaseA := make(chan struct{})
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/subscriptions" {
			http.Error(w, "unexpected provider operation", http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("customer") == "cus_a" {
			select {
			case <-enteredA:
			default:
				close(enteredA)
			}
			<-releaseA
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"has_more":false}`))
	}))
	defer provider.Close()
	svc := &billing.Service{Store: db, Config: billing.Config{BuilderPrice: "price_builder", ScalePrice: "price_scale", BaseURL: provider.URL}}
	resultA := make(chan error, 1)
	go func() {
		_, err := svc.ReconcileCustomer(context.Background(), accountA)
		resultA <- err
	}()
	select {
	case <-enteredA:
	case <-time.After(time.Second):
		t.Fatal("account A did not reach the held provider call")
	}
	resultB := make(chan error, 1)
	go func() {
		_, err := svc.ReconcileCustomer(context.Background(), accountB)
		resultB <- err
	}()
	select {
	case err := <-resultB:
		if err != nil {
			t.Fatalf("account B reconcile: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("account B was blocked by account A's provider call")
	}
	close(releaseA)
	if err := <-resultA; err != nil {
		t.Fatalf("account A reconcile: %v", err)
	}
}

func TestCheckoutAndClosureSerializeForSameAccount(t *testing.T) {
	db := openBillingStore(t)
	account := createBillingAccount(t, db, "same@example.test", "cus_same")
	firstList := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondList := make(chan struct{})
	var lists atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/subscriptions":
			n := lists.Add(1)
			if n == 1 {
				close(firstList)
				<-releaseFirst
			} else if n == 2 {
				close(secondList)
			}
			_, _ = w.Write([]byte(`{"data":[],"has_more":false}`))
		case r.Method == http.MethodPost && r.URL.Path == "/checkout/sessions":
			_, _ = w.Write([]byte(`{"id":"cs_test","url":"https://checkout.stripe.com/test"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/checkout/sessions/cs_test":
			_, _ = w.Write([]byte(`{"id":"cs_test","status":"expired"}`))
		default:
			http.Error(w, "unexpected provider operation", http.StatusNotFound)
		}
	}))
	defer provider.Close()
	svc := &billing.Service{Store: db, Config: billing.Config{BuilderPrice: "price_builder", ScalePrice: "price_scale", Origin: "https://app.example.test", BaseURL: provider.URL}}
	checkout := make(chan error, 1)
	go func() {
		_, err := svc.Checkout(context.Background(), account, "builder")
		checkout <- err
	}()
	select {
	case <-firstList:
	case <-time.After(time.Second):
		t.Fatal("checkout did not reach provider")
	}
	closure := make(chan error, 1)
	go func() { closure <- svc.CancelAccount(context.Background(), account) }()
	select {
	case <-secondList:
		t.Fatal("closure reached provider while checkout still held the account lock")
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseFirst)
	if err := <-checkout; err != nil {
		t.Fatalf("checkout: %v", err)
	}
	select {
	case <-secondList:
	case <-time.After(time.Second):
		t.Fatal("closure did not continue after checkout released the lock")
	}
	if err := <-closure; err != nil {
		t.Fatalf("closure: %v", err)
	}
}

func TestWebhookRefetchesAfterReconcileWinsAccountLock(t *testing.T) {
	db := openBillingStore(t)
	account := createBillingAccount(t, db, "ordering@example.test", "cus_ordering")
	firstFetch := make(chan struct{})
	releaseFirst := make(chan struct{})
	var subscriptionReads atomic.Int32
	now := time.Now().UTC()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/subscriptions" {
			_, _ = w.Write([]byte(`{"data":[],"has_more":false}`))
			return
		}
		if r.URL.Path != "/subscriptions/sub_ordering" {
			http.Error(w, "unexpected provider operation", http.StatusNotFound)
			return
		}
		read := subscriptionReads.Add(1)
		status := "canceled"
		if read == 1 {
			close(firstFetch)
			<-releaseFirst
			status = "active" // Deliberately stale; this snapshot may identify only the lock key.
		}
		_, _ = fmt.Fprintf(w, `{"id":"sub_ordering","customer":"cus_ordering","status":%q,"items":{"data":[{"price":{"id":"price_builder"},"current_period_start":%d,"current_period_end":%d}]}}`, status, now.Unix(), now.Add(24*time.Hour).Unix())
	}))
	defer provider.Close()
	svc := &billing.Service{Store: db, Config: billing.Config{WebhookSecret: "secret", BuilderPrice: "price_builder", ScalePrice: "price_scale", BaseURL: provider.URL}}
	body, err := json.Marshal(map[string]any{
		"id": "evt_ordering", "type": "customer.subscription.updated", "created": time.Now().Unix(),
		"data": map[string]any{"object": map[string]any{"id": "sub_ordering"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	webhook := make(chan error, 1)
	go func() { webhook <- svc.Webhook(context.Background(), body, signature(body, "secret", time.Now())) }()
	select {
	case <-firstFetch:
	case <-time.After(time.Second):
		t.Fatal("webhook did not start identity fetch")
	}
	reconciled := make(chan error, 1)
	go func() {
		_, reconcileErr := svc.ReconcileCustomer(context.Background(), account)
		reconciled <- reconcileErr
	}()
	select {
	case err = <-reconciled:
		if err != nil {
			t.Fatalf("reconcile during webhook identity fetch: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pre-lock webhook provider read blocked same-account reconcile")
	}
	close(releaseFirst)
	if err = <-webhook; err != nil {
		t.Fatalf("webhook: %v", err)
	}
	var status, plan string
	if err = db.DB().QueryRowContext(context.Background(), `SELECT s.status,a.plan_id FROM subscriptions s JOIN accounts a ON a.id=s.account_id WHERE s.id='sub_ordering'`).Scan(&status, &plan); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if status != "canceled" || plan != "free" {
		t.Fatalf("stale webhook projection won: status=%q plan=%q reads=%d", status, plan, subscriptionReads.Load())
	}
	if subscriptionReads.Load() != 2 {
		t.Fatalf("want identity-only and locked authoritative reads, got %d", subscriptionReads.Load())
	}
}

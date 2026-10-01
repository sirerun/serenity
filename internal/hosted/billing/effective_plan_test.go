package billing_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/billing"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func TestBillingAccountPlanTracksEffectiveSubscriptionAccess(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	tests := []struct {
		name, status, accountStatus string
		periodEnd                   time.Time
		failureAt                   time.Time
		wantPlan                    string
		wantEligible                bool
		wantFrozen                  bool
	}{
		{name: "canceled", status: "canceled", periodEnd: now.Add(24 * time.Hour), wantPlan: "free"},
		{name: "unpaid", status: "unpaid", periodEnd: now.Add(24 * time.Hour), wantPlan: "free"},
		{name: "paused", status: "paused", periodEnd: now.Add(24 * time.Hour), wantPlan: "free"},
		{name: "active expired period", status: "active", periodEnd: now.Add(-time.Hour), wantPlan: "free"},
		{name: "active future period", status: "active", periodEnd: now.Add(24 * time.Hour), wantPlan: "builder", wantEligible: true},
		{name: "past due expired grace", status: "past_due", periodEnd: now.Add(24 * time.Hour), failureAt: now.Add(-96 * time.Hour), wantPlan: "free"},
		{name: "past due future grace", status: "past_due", periodEnd: now.Add(24 * time.Hour), failureAt: now.Add(-24 * time.Hour), wantPlan: "builder", wantEligible: true},
		{name: "deleting remains free", status: "active", accountStatus: "deleting", periodEnd: now.Add(24 * time.Hour), wantPlan: "free", wantFrozen: true},
		{name: "restore pending remains free", status: "active", accountStatus: "restore_pending", periodEnd: now.Add(24 * time.Hour), wantPlan: "free", wantFrozen: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			db := openBillingStore(t)
			account := createBillingAccount(t, db, "effective-"+tt.name+"@example.test", "cus_effective")
			if tt.accountStatus != "" {
				if _, err := db.DB().ExecContext(ctx, `UPDATE accounts SET status=? WHERE id=?`, tt.accountStatus, account); err != nil {
					t.Fatal(err)
				}
			}
			if tt.status == "past_due" {
				_, err := db.DB().ExecContext(ctx, `INSERT INTO billing_failures(account_id,subscription_id,invoice_id,event_id,first_failed_at) VALUES(?,?,?,?,?)`, account, "sub_effective", "in_effective", "evt_payment_failure", store.Stamp(tt.failureAt))
				if err != nil {
					t.Fatal(err)
				}
			}
			sub := map[string]any{
				"id": "sub_effective", "customer": "cus_effective", "status": tt.status,
				"items": map[string]any{"data": []any{map[string]any{"price": map[string]any{"id": "price_builder"}, "current_period_start": now.Add(-24 * time.Hour).Unix(), "current_period_end": tt.periodEnd.Unix()}}},
			}
			if tt.status == "past_due" {
				sub["latest_invoice"] = "in_effective"
			}
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/subscriptions/sub_effective":
					_ = json.NewEncoder(w).Encode(sub)
				case "/subscriptions":
					_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{sub}, "has_more": false})
				default:
					http.Error(w, "unexpected provider operation: "+r.URL.Path, http.StatusNotFound)
				}
			}))
			defer provider.Close()
			svc := &billing.Service{Store: db, Config: billing.Config{WebhookSecret: "secret", BuilderPrice: "price_builder", ScalePrice: "price_scale", BaseURL: provider.URL}}
			body, err := json.Marshal(map[string]any{"id": "evt_effective", "type": "customer.subscription.updated", "created": now.Unix(), "data": map[string]any{"object": map[string]any{"id": "sub_effective"}}})
			if err != nil {
				t.Fatal(err)
			}
			if err = svc.Webhook(ctx, body, signature(body, "secret", now)); err != nil {
				t.Fatalf("webhook: %v", err)
			}
			assertAccountPlan(t, db, account, tt.wantPlan)

			result, reconcileErr := svc.ReconcileCustomer(ctx, account)
			if tt.wantFrozen {
				if !errors.Is(reconcileErr, contracts.ErrBillingAccountFrozen) {
					t.Fatalf("reconcile error = %v, want frozen", reconcileErr)
				}
			} else if reconcileErr != nil {
				t.Fatalf("reconcile: %v", reconcileErr)
			}
			assertAccountPlan(t, db, account, tt.wantPlan)
			if !tt.wantFrozen && result.Eligible != tt.wantEligible {
				t.Fatalf("reconcile Eligible=%v, want %v (result=%+v)", result.Eligible, tt.wantEligible, result)
			}
			if tt.status != "canceled" && result.PlanID != "builder" {
				t.Fatalf("provider plan provenance lost: %+v", result)
			}
		})
	}
}

func assertAccountPlan(t *testing.T, db *store.Store, account, want string) {
	t.Helper()
	var plan string
	if err := db.DB().QueryRowContext(context.Background(), `SELECT plan_id FROM accounts WHERE id=?`, account).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	if plan != want {
		t.Fatalf("account plan=%q, want %q", plan, want)
	}
}

func TestReconcileMalformedPersistedGraceRollsBack(t *testing.T) {
	ctx := context.Background()
	db := openBillingStore(t)
	account := createBillingAccount(t, db, "bad-grace@example.test", "cus_bad_grace")
	periodStart := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Second)
	periodEnd := periodStart.Add(30 * 24 * time.Hour)
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO subscriptions(id,account_id,price_id,plan_id,status,current_period_start,current_period_end,grace_until,grace_invoice_id) VALUES('sub_bad_grace',?,'price_builder','builder','past_due',?,?,'malformed','in_old')`, account, store.Stamp(periodStart), store.Stamp(periodEnd)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().ExecContext(ctx, `INSERT INTO billing_failures(account_id,subscription_id,invoice_id,event_id,first_failed_at) VALUES(?,?,?,?,?)`, account, "sub_bad_grace", "in_new", "evt_new_failure", store.Stamp(time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"data":[{"id":"sub_bad_grace","customer":"cus_bad_grace","status":"past_due","latest_invoice":"in_new","items":{"data":[{"price":{"id":"price_builder"},"current_period_start":%d,"current_period_end":%d}]}}],"has_more":false}`, periodStart.Unix(), periodEnd.Unix())
	}))
	defer provider.Close()
	svc := &billing.Service{Store: db, Config: billing.Config{BuilderPrice: "price_builder", ScalePrice: "price_scale", BaseURL: provider.URL}}
	if _, err := svc.ReconcileCustomer(ctx, account); err == nil {
		t.Fatal("malformed persisted grace was accepted")
	}
	var status, grace, invoice string
	if err := db.DB().QueryRowContext(ctx, `SELECT status,grace_until,grace_invoice_id FROM subscriptions WHERE id='sub_bad_grace'`).Scan(&status, &grace, &invoice); err != nil {
		t.Fatal(err)
	}
	if status != "past_due" || grace != "malformed" || invoice != "in_old" {
		t.Fatalf("malformed grace transaction partially committed: status=%q grace=%q invoice=%q", status, grace, invoice)
	}
}

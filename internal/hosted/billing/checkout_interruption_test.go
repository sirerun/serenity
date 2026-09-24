package billing_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/billing"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func TestClosureRecoversCheckoutAfterLocalSaveFailure(t *testing.T) {
	checkInterruptedCheckout(t, false)
}
func TestReconcileRecoversCheckoutAfterLocalSaveFailure(t *testing.T) {
	checkInterruptedCheckout(t, true)
}
func checkInterruptedCheckout(t *testing.T, reconcileFirst bool) {
	t.Helper()
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
	account, err := db.CreateAccount(ctx, "interruption@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_interrupted' WHERE id=?`, account.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.DB().ExecContext(ctx, `CREATE TRIGGER fail_session_save BEFORE UPDATE OF session_id ON checkout_attempts BEGIN SELECT RAISE(ABORT,'injected session save failure'); END`); err != nil {
		t.Fatal(err)
	}
	providerOpen := false
	attemptID := ""
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /subscriptions":
			if providerOpen && reconcileFirst {
				_, _ = fmt.Fprintf(w, `{"data":[{"id":"sub_other","customer":"cus_interrupted","status":"active","items":{"data":[{"price":{"id":"price_builder"},"current_period_start":%d,"current_period_end":%d}]}}],"has_more":false}`, time.Now().Unix(), time.Now().Add(24*time.Hour).Unix())
			} else {
				_, _ = fmt.Fprint(w, `{"data":[],"has_more":false}`)
			}
		case "POST /checkout/sessions":
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			attemptID = r.Form.Get("metadata[serenity_attempt]")
			if attemptID == "" || r.Form.Get("subscription_data[metadata][serenity_attempt]") != attemptID {
				t.Error("missing stable opaque attempt metadata")
			}
			providerOpen = true
			_, _ = fmt.Fprint(w, `{"id":"cs_interrupted","url":"https://checkout.stripe.com/fixture","status":"open"}`)
		case "GET /checkout/sessions":
			if providerOpen {
				_, _ = fmt.Fprintf(w, `{"data":[{"id":"cs_interrupted","url":"https://checkout.stripe.com/fixture","status":"open","mode":"subscription","customer":"cus_interrupted","client_reference_id":%q,"metadata":{"serenity_account":%q,"serenity_attempt":%q}}],"has_more":false}`, account.ID, account.ID, attemptID)
			} else {
				_, _ = fmt.Fprint(w, `{"data":[],"has_more":false}`)
			}
		case "GET /checkout/sessions/cs_interrupted":
			if providerOpen {
				_, _ = fmt.Fprint(w, `{"id":"cs_interrupted","status":"open"}`)
			} else {
				_, _ = fmt.Fprint(w, `{"id":"cs_interrupted","status":"expired"}`)
			}
		case "POST /checkout/sessions/cs_interrupted/expire":
			providerOpen = false
			_, _ = fmt.Fprint(w, `{"id":"cs_interrupted","status":"expired"}`)
		case "DELETE /subscriptions/sub_other":
			_, _ = fmt.Fprint(w, `{"id":"sub_other","status":"canceled"}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", 500)
		}
	}))
	t.Cleanup(provider.Close)
	cfg := billing.Config{BaseURL: provider.URL, BuilderPrice: "price_builder"}
	s := &billing.Service{Store: db, Config: cfg}
	if _, err = s.Checkout(ctx, account.ID, "builder"); err == nil {
		t.Fatal("injected local save failure did not occur")
	}
	if !providerOpen {
		t.Fatal("provider did not create checkout")
	}
	var id string
	if err = db.DB().QueryRowContext(ctx, `SELECT COALESCE(session_id,'') FROM checkout_attempts WHERE account_id=?`, account.ID).Scan(&id); err != nil || id != "" {
		t.Fatalf("missing-ID fixture=%q err=%v", id, err)
	}
	if _, err = db.DB().ExecContext(ctx, `DROP TRIGGER fail_session_save`); err != nil {
		t.Fatal(err)
	}
	if reconcileFirst {
		if _, err = s.ReconcileCustomer(ctx, account.ID); err != nil {
			t.Fatalf("provider session was not recovered during reconciliation: %v", err)
		}
		var recovered string
		if err = db.DB().QueryRowContext(ctx, `SELECT COALESCE(session_id,'') FROM checkout_attempts WHERE account_id=?`, account.ID).Scan(&recovered); err != nil || recovered != "cs_interrupted" {
			t.Fatalf("recovered session=%q err=%v", recovered, err)
		}
	}
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET status='deleting' WHERE id=?`, account.ID); err != nil {
		t.Fatal(err)
	}
	s = &billing.Service{Store: db, Config: cfg}
	result, err := s.CloseBillingAccount(ctx, account.ID)
	if err != nil || result.Status != contracts.CloseStatusClosed || providerOpen {
		t.Fatalf("recovered checkout was not expired before closure: %+v err=%v open=%v", result, err, providerOpen)
	}
	var retained int
	if err = db.DB().QueryRowContext(ctx, `SELECT count(*) FROM checkout_attempts WHERE account_id=?`, account.ID).Scan(&retained); err != nil || retained != 0 {
		t.Fatalf("attempts retained after certified closure=%d err=%v", retained, err)
	}
}

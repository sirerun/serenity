package billing_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/billing"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func TestClosureAfterCheckoutProviderSuccessLocalSaveFailure(t *testing.T) {
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
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /subscriptions":
			_, _ = fmt.Fprint(w, `{"data":[],"has_more":false}`)
		case "POST /checkout/sessions":
			providerOpen = true
			_, _ = fmt.Fprint(w, `{"id":"cs_interrupted","url":"https://checkout.stripe.com/fixture","status":"open"}`)
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
	if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET status='deleting' WHERE id=?`, account.ID); err != nil {
		t.Fatal(err)
	}
	s = &billing.Service{Store: db, Config: cfg}
	result, err := s.CloseBillingAccount(ctx, account.ID)
	if result.Status == contracts.CloseStatusClosed && providerOpen {
		t.Fatalf("billing certified closed while provider checkout remains open: result=%+v err=%v", result, err)
	}
	if result.Status != contracts.CloseStatusPending || !errors.Is(err, contracts.ErrBillingProviderAmbiguous) {
		t.Fatalf("expected pending ambiguity: %+v %v", result, err)
	}
	var retained int
	if err = db.DB().QueryRowContext(ctx, `SELECT count(*) FROM checkout_attempts WHERE account_id=?`, account.ID).Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("attempt retained=%d err=%v", retained, err)
	}

}

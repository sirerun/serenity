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

func TestLegacyCancelAccountRejectsPendingClosure(t *testing.T) {
	for _, returnedStatus := range []string{"active", "canceled"} {
		t.Run(returnedStatus, func(t *testing.T) {
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
			a, err := db.CreateAccount(ctx, "legacy@example.com")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_legacy',plan_id='builder' WHERE id=?`, a.ID); err != nil {
				t.Fatal(err)
			}
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method + " " + r.URL.Path {
				case "GET /subscriptions":
					_, _ = fmt.Fprint(w, `{"data":[{"id":"sub_legacy","customer":"cus_legacy","status":"active","items":{"data":[{"price":{"id":"price_builder"}}]}}],"has_more":false}`)
				case "DELETE /subscriptions/sub_legacy":
					_, _ = fmt.Fprintf(w, `{"id":"sub_legacy","customer":"cus_legacy","status":%q}`, returnedStatus)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected", 500)
				}
			}))
			t.Cleanup(provider.Close)
			s := &billing.Service{Store: db, Config: billing.Config{BaseURL: provider.URL, BuilderPrice: "price_builder"}}
			if err = s.CancelAccount(ctx, a.ID); !errors.Is(err, contracts.ErrBillingProviderAmbiguous) {
				t.Fatalf("legacy adapter reported successful cancellation while closure is pending: %v", err)
			}
			var plan string
			if err = db.DB().QueryRowContext(ctx, `SELECT plan_id FROM accounts WHERE id=?`, a.ID).Scan(&plan); err != nil || plan != "builder" {
				t.Fatalf("pending closure changed plan: %s %v", plan, err)
			}
		})
	}
}

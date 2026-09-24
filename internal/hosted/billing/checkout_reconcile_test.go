package billing_test

import (
	"context"
	"errors"
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

func TestOldCheckoutRequiresConfirmedExpiration(t *testing.T) {
	for _, tc := range []struct {
		name, id, status string
		accepted         bool
	}{
		{"still_open", "cs_old", "open", false},
		{"completed_instead", "cs_old", "complete", false},
		{"wrong_session", "cs_other", "expired", false},
		{"confirmed", "cs_old", "expired", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			a, err := db.CreateAccount(ctx, "stale@example.com")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_stale' WHERE id=?`, a.ID); err != nil {
				t.Fatal(err)
			}
			if _, err = db.DB().ExecContext(ctx, `INSERT INTO checkout_attempts(account_id,id,price_id,session_id,created_at) VALUES(?,?,?,?,?)`, a.ID, "attempt", "price_builder", "cs_old", store.Stamp(time.Now().Add(-25*time.Hour))); err != nil {
				t.Fatal(err)
			}
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method + " " + r.URL.Path {
				case "GET /subscriptions":
					_, _ = fmt.Fprint(w, `{"data":[],"has_more":false}`)
				case "GET /checkout/sessions/cs_old":
					_, _ = fmt.Fprint(w, `{"id":"cs_old","status":"open"}`)
				case "POST /checkout/sessions/cs_old/expire":
					_, _ = fmt.Fprintf(w, `{"id":%q,"status":%q}`, tc.id, tc.status)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected", 500)
				}
			}))
			t.Cleanup(provider.Close)
			s := &billing.Service{Store: db, Config: billing.Config{BaseURL: provider.URL, BuilderPrice: "price_builder"}}
			_, err = s.ReconcileCustomer(ctx, a.ID)
			if tc.accepted {
				if err != nil {
					t.Fatal(err)
				}
			} else if !errors.Is(err, contracts.ErrBillingProviderAmbiguous) {
				t.Fatalf("unconfirmed expiration error=%v", err)
			}
			var attempts, audits int
			if err = db.DB().QueryRowContext(ctx, `SELECT count(*) FROM checkout_attempts WHERE account_id=?`, a.ID).Scan(&attempts); err != nil {
				t.Fatal(err)
			}
			if err = db.DB().QueryRowContext(ctx, `SELECT count(*) FROM audit_log WHERE account_id=? AND action='checkout_attempt_reconciled'`, a.ID).Scan(&audits); err != nil {
				t.Fatal(err)
			}
			if tc.accepted {
				if attempts != 0 || audits != 1 {
					t.Fatalf("attempts=%d audits=%d", attempts, audits)
				}
			} else if attempts != 1 || audits != 0 {
				t.Fatalf("unconfirmed cleanup: attempts=%d audits=%d", attempts, audits)
			}
		})
	}
}

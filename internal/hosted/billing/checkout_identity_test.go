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
	"github.com/sirerun/serenity/internal/hosted/store"
)

func TestCheckoutRetryRejectsUnmatchedSession(t *testing.T) {
	for _, status := range []string{"open", "expired"} {
		t.Run(status, func(t *testing.T) {
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
			a, err := db.CreateAccount(ctx, "retry@example.com")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.DB().ExecContext(ctx, `UPDATE accounts SET stripe_customer_id='cus_retry' WHERE id=?`, a.ID); err != nil {
				t.Fatal(err)
			}
			if _, err = db.DB().ExecContext(ctx, `INSERT INTO checkout_attempts(account_id,id,price_id,session_id,created_at) VALUES(?,?,?,?,?)`, a.ID, "attempt", "price_builder", "cs_expected", store.Stamp(time.Now())); err != nil {
				t.Fatal(err)
			}
			created := 0
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method + " " + r.URL.Path {
				case "GET /subscriptions":
					_, _ = fmt.Fprint(w, `{"data":[],"has_more":false}`)
				case "GET /checkout/sessions/cs_expected":
					_, _ = fmt.Fprintf(w, `{"id":"cs_other","status":%q,"url":"https://checkout.stripe.com/other"}`, status)
				case "POST /checkout/sessions":
					created++
					_, _ = fmt.Fprint(w, `{"id":"cs_new","status":"open","url":"https://checkout.stripe.com/new"}`)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected", 500)
				}
			}))
			t.Cleanup(provider.Close)
			s := &billing.Service{Store: db, Config: billing.Config{BaseURL: provider.URL, BuilderPrice: "price_builder"}}
			location, err := s.Checkout(ctx, a.ID, "builder")
			if err == nil || location != "" {
				t.Fatalf("unmatched session accepted: url=%q err=%v", location, err)
			}
			if created != 0 {
				t.Fatalf("created %d replacement sessions", created)
			}
			var id, session string
			if err := db.DB().QueryRowContext(ctx, `SELECT id,session_id FROM checkout_attempts WHERE account_id=?`, a.ID).Scan(&id, &session); err != nil {
				t.Fatal(err)
			}
			if id != "attempt" || session != "cs_expected" {
				t.Fatalf("attempt changed: %s %s", id, session)
			}
		})
	}
}

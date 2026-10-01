package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/billing"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/store"
)

type deletionEmbedding struct{}

func (deletionEmbedding) ModelVersion() string { return "test@v1" }
func (deletionEmbedding) Embed(context.Context, string) ([]float32, error) {
	return []float32{1, 1, 1}, nil
}

type deletionCloser func(context.Context, string) (contracts.CloseResult, error)

func (f deletionCloser) CloseBillingAccount(ctx context.Context, id string) (contracts.CloseResult, error) {
	return f(ctx, id)
}

func deletionFixture(t *testing.T) (*store.Store, Config, string, string) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	account, err := db.CreateAccount(context.Background(), "delete@example.test")
	if err != nil {
		t.Fatal(err)
	}
	brainID := store.ID()
	if _, err = db.DB().Exec(`INSERT INTO brains(id,account_id,path_key,state,created_at) VALUES(?,?,?,'ready',?)`, brainID, account.ID, brainID, store.Stamp(time.Now())); err != nil {
		t.Fatal(err)
	}
	brainPath := filepath.Join(dir, "brains", brainID)
	if err = os.MkdirAll(brainPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(brainPath, "retained.txt"), []byte("must remain until closure"), 0600); err != nil {
		t.Fatal(err)
	}
	return db, Config{DataDir: dir, MaxOpen: 2, MaxInFlight: 4, PublicOrigin: "http://localhost", AccountCap: 100}, account.ID, brainPath
}

func TestDeleteAccountFreezesBeforeClosureAndRetainsUntilClosed(t *testing.T) {
	db, cfg, accountID, path := deletionFixture(t)
	s, err := Assemble(cfg, true, db, nil, deletionEmbedding{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Pool.Close(); s.Gateway.Close() })
	closed := false
	s.billingCloser = deletionCloser(func(ctx context.Context, id string) (contracts.CloseResult, error) {
		var status string
		if err := db.DB().QueryRowContext(ctx, `SELECT status FROM accounts WHERE id=?`, id).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "deleting" {
			t.Fatalf("provider closure called before durable freeze: %q", status)
		}
		if closed {
			return contracts.CloseResult{Status: contracts.CloseStatusClosed}, nil
		}
		return contracts.CloseResult{Status: contracts.CloseStatusPending}, contracts.ErrBillingProviderUnavailable
	})
	if err = s.DeleteAccount(context.Background(), accountID); !errors.Is(err, contracts.ErrBillingProviderUnavailable) {
		t.Fatalf("uncertain closure = %v", err)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatalf("brain erased before closure: %v", err)
	}
	closed = true
	if err = s.DeleteAccount(context.Background(), accountID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("brain retained after certified closure: %v", err)
	}
}

func TestStartupDeletionClosesBillingBeforePurging(t *testing.T) {
	for _, unavailable := range []bool{true, false} {
		t.Run(map[bool]string{true: "unavailable", false: "closed"}[unavailable], func(t *testing.T) {
			db, cfg, accountID, path := deletionFixture(t)
			if _, err := db.DB().Exec(`UPDATE accounts SET status='deleting',stripe_customer_id='cus_startup' WHERE id=?`, accountID); err != nil {
				t.Fatal(err)
			}
			calls := 0
			provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if _, err := os.Stat(path); err != nil {
					t.Errorf("startup purged before provider closure: %v", err)
				}
				if unavailable {
					http.Error(w, "unavailable", http.StatusServiceUnavailable)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"data":[],"has_more":false}`))
			}))
			defer provider.Close()
			cfg.BillingEnabled = true
			cfg.billingConfig = &billing.Config{BaseURL: provider.URL, BuilderPrice: "price_builder", ScalePrice: "price_scale"}
			s, err := Assemble(cfg, true, db, nil, deletionEmbedding{})
			if s != nil {
				defer func() { s.Gateway.Close(); _ = s.Pool.Close() }()
			}
			if calls == 0 {
				t.Fatal("startup did not certify provider closure")
			}
			if unavailable {
				if err == nil {
					t.Fatal("startup accepted uncertain billing closure")
				}
				if _, err = os.Stat(path); err != nil {
					t.Fatalf("uncertain startup erased brain: %v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if _, err = os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("closed startup retained brain: %v", err)
				}
			}
		})
	}
}

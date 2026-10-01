package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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
	return db, Config{DataDir: dir, MaxOpen: 2, MaxInFlight: 4, PublicOrigin: "http://127.0.0.1", AccountCap: 100}, account.ID, brainPath
}

func TestDeleteAccountFreezesBeforeClosureAndRetainsUntilClosed(t *testing.T) {
	db, cfg, accountID, path := deletionFixture(t)
	s, err := Assemble(cfg, true, db, nil, deletionEmbedding{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Pool.Close(); s.Gateway.Close() })
	closed := false
	providerErr := error(contracts.ErrBillingProviderUnavailable)
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
		return contracts.CloseResult{Status: contracts.CloseStatusPending}, providerErr
	})
	if err = s.DeleteAccount(context.Background(), accountID); !errors.Is(err, contracts.ErrBillingProviderUnavailable) {
		t.Fatalf("uncertain closure = %v", err)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatalf("brain erased before closure: %v", err)
	}
	providerErr = nil
	if err = s.DeleteAccount(context.Background(), accountID); err == nil {
		t.Fatal("pending closure without an error allowed purge")
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatalf("pending closure erased brain: %v", err)
	}
	closed = true
	if err = s.DeleteAccount(context.Background(), accountID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("brain retained after certified closure: %v", err)
	}
}

func TestAccountDeletionMaintenanceFenceBlocksBackupUntilPurgeCompletes(t *testing.T) {
	db, cfg, accountID, brainPath := deletionFixture(t)
	s, err := Assemble(cfg, true, db, nil, deletionEmbedding{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Pool.Close(); s.Gateway.Close() })
	if _, err = db.DB().Exec(`UPDATE accounts SET stripe_customer_id='cus_fence' WHERE id=?`, accountID); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	releaseClosure := make(chan struct{})
	var releaseOnce sync.Once
	releaseProvider := func() { releaseOnce.Do(func() { close(releaseClosure) }) }
	t.Cleanup(releaseProvider)
	s.billingCloser = deletionCloser(func(context.Context, string) (contracts.CloseResult, error) {
		close(entered)
		<-releaseClosure
		return contracts.CloseResult{Status: contracts.CloseStatusClosed}, nil
	})
	deleteDone := make(chan error, 1)
	go func() { deleteDone <- s.DeleteAccount(context.Background(), accountID) }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("billing preflight did not start")
	}
	if s.Gateway.Maintenance.TryLock() {
		s.Gateway.Maintenance.Unlock()
		t.Fatal("maintenance write lock was available during provider closure")
	}

	snapshot := filepath.Join(cfg.DataDir, "fenced-snapshot")
	backupStarted := make(chan struct{})
	backupDone := make(chan error, 1)
	go func() {
		close(backupStarted)
		backupDone <- s.Backup(context.Background(), snapshot)
	}()
	<-backupStarted
	queueDeadline := time.NewTimer(5 * time.Second)
	defer queueDeadline.Stop()
	for {
		if !s.Gateway.Maintenance.TryRLock() {
			break // the backup's exclusive lock is queued behind deletion's read lock
		}
		s.Gateway.Maintenance.RUnlock()
		runtime.Gosched()
		select {
		case err = <-backupDone:
			t.Fatalf("backup completed while deletion was between freeze and purge: %v", err)
		case <-queueDeadline.C:
			t.Fatal("backup did not queue behind the account deletion fence")
		default:
		}
	}
	if _, err = os.Stat(brainPath); err != nil {
		t.Fatalf("brain was purged before provider closure completed: %v", err)
	}

	releaseProvider()
	select {
	case err = <-deleteDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("account and brain helpers deadlocked behind the queued backup writer")
	}
	select {
	case err = <-backupDone:
		if err != nil {
			t.Fatalf("queued backup failed after deletion released its fence: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("queued backup did not acquire the maintenance fence after deletion")
	}
	if _, err = os.Stat(brainPath); !os.IsNotExist(err) {
		t.Fatalf("brain remains after completed deletion: %v", err)
	}
	snapshotDB, err := store.Open(filepath.Join(snapshot, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = snapshotDB.Close() }()
	var state string
	if err = snapshotDB.DB().QueryRow(`SELECT status FROM accounts WHERE id=?`, accountID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "deleted" {
		t.Fatalf("snapshot captured account between freeze and purge: status=%q", state)
	}
}

func TestDeletionWithoutConfiguredCloserRetainsExistingCustomer(t *testing.T) {
	db, cfg, accountID, path := deletionFixture(t)
	s, err := Assemble(cfg, true, db, nil, deletionEmbedding{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Gateway.Close(); _ = s.Pool.Close() }()
	if _, err = db.DB().Exec(`UPDATE accounts SET stripe_customer_id='cus_prior_configuration' WHERE id=?`, accountID); err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteAccount(context.Background(), accountID); err == nil {
		t.Fatal("billing-disabled config ignored persisted provider customer")
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatalf("unconfigured closer erased brain: %v", err)
	}
}

func TestDashboardPendingDeletionFreezesAndRetriesRetainingMemory(t *testing.T) {
	db, cfg, accountID, path := deletionFixture(t)
	s, err := Assemble(cfg, true, db, nil, deletionEmbedding{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Gateway.Close(); _ = s.Pool.Close() }()
	token := strings.Repeat("s", 43)
	now := time.Now()
	if _, err = db.DB().Exec(`INSERT INTO sessions(id,account_id,token_hash,created_at,expires_at,csrf_secret) VALUES(?,?,?,?,?,?)`, "session_fixture", accountID, store.Hash(token), store.Stamp(now), store.Stamp(now.Add(time.Hour)), "csrf_fixture"); err != nil {
		t.Fatal(err)
	}
	closed := false
	s.billingCloser = deletionCloser(func(ctx context.Context, id string) (contracts.CloseResult, error) {
		var status string
		if err := db.DB().QueryRowContext(ctx, `SELECT status FROM accounts WHERE id=?`, id).Scan(&status); err != nil {
			return contracts.CloseResult{}, err
		}
		if status != "deleting" {
			return contracts.CloseResult{}, errors.New("provider called before freeze")
		}
		if closed {
			return contracts.CloseResult{Status: contracts.CloseStatusClosed}, nil
		}
		return contracts.CloseResult{Status: contracts.CloseStatusPending}, nil
	})
	request := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/account/delete", strings.NewReader(url.Values{"confirm": {"DELETE"}, "csrf": {"csrf_fixture"}}.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", cfg.PublicOrigin)
		req.AddCookie(&http.Cookie{Name: "serenity_session", Value: token})
		response := httptest.NewRecorder()
		s.Handler.ServeHTTP(response, req)
		return response
	}
	if response := request(); response.Code != http.StatusServiceUnavailable {
		t.Fatalf("pending deletion response=%d %s", response.Code, response.Body.String())
	}
	var status string
	if err = db.DB().QueryRow(`SELECT status FROM accounts WHERE id=?`, accountID).Scan(&status); err != nil || status != "deleting" {
		t.Fatalf("pending deletion status=%q err=%v", status, err)
	}
	if _, err = os.Stat(path); err != nil {
		t.Fatalf("pending dashboard deletion erased memory: %v", err)
	}
	closed = true
	if response := request(); response.Code != http.StatusOK {
		t.Fatalf("deletion retry response=%d %s", response.Code, response.Body.String())
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("certified dashboard deletion retained memory: %v", err)
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

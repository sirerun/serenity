package service

import (
	"context"
	"errors"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"os"
	"testing"
)

func TestStartupRejectsNilContextBeforeDependencies(t *testing.T) {
	if _, err := NewWithDependencies(nil, Config{}, true, nil, LifecycleDependencies{}); !errors.Is(err, ErrStartupUnavailable) {
		t.Fatalf("nil startup context: %v", err)
	}
	if _, err := AssembleWithDependencies(nil, Config{}, true, nil, nil, nil, LifecycleDependencies{}); !errors.Is(err, ErrStartupUnavailable) {
		t.Fatalf("nil assembly context: %v", err)
	}
}
func TestRestorePendingPreflightRequiresAdmittedReplay(t *testing.T) {
	db, cfg, accountID, path := deletionFixture(t)
	if _, err := db.DB().Exec("UPDATE accounts SET status='restore_pending' WHERE id=?", accountID); err != nil {
		t.Fatal(err)
	}
	s := &Service{Store: db, cfg: cfg}
	calls := 0
	closed := false
	s.billingCloser = deletionCloser(func(ctx context.Context, id string) (contracts.CloseResult, error) {
		calls++
		var status string
		if err := db.DB().QueryRowContext(ctx, "SELECT status FROM accounts WHERE id=?", id).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "deleting" {
			t.Fatalf("provider called before replay freeze: %s", status)
		}
		if closed {
			return contracts.CloseResult{Status: contracts.CloseStatusClosed}, nil
		}
		return contracts.CloseResult{Status: contracts.CloseStatusPending}, nil
	})
	if _, err := s.accountDeletionPreflight(context.Background(), accountID); !errors.Is(err, contracts.ErrBillingAccountFrozen) {
		t.Fatalf("ordinary frozen delete: %v", err)
	}
	if calls != 0 {
		t.Fatalf("ordinary delete called provider %d times", calls)
	}
	if proceed, err := s.deletionPreflight(context.Background(), accountID, true); err == nil || proceed {
		t.Fatalf("uncertain replay closure: proceed=%v err=%v", proceed, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("pending provider closure lost files: %v", err)
	}
	closed = true
	if proceed, err := s.deletionPreflight(context.Background(), accountID, true); err != nil || !proceed {
		t.Fatalf("confirmed replay closure: proceed=%v err=%v", proceed, err)
	}
}

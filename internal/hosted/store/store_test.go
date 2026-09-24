package store_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/store"
)

func TestBrainByID(t *testing.T) {
	ctx := context.Background()
	s, e := store.Open(filepath.Join(t.TempDir(), "control.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	a, e := s.CreateAccount(ctx, "a@example.com")
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.CreateAccount(ctx, "b@example.com")
	if e != nil {
		t.Fatal(e)
	}
	id := store.ID()
	if _, e = s.InsertBrain(ctx, a.ID, id, id, "ready", time.Now()); e != nil {
		t.Fatal(e)
	}
	if _, e = s.BrainByID(ctx, b.ID, id); !errors.Is(e, store.ErrNotFound) {
		t.Fatalf("cross-account read: %v", e)
	}
	c := store.ClientCredential{ID: store.ID(), BrainID: id, AccountID: b.ID, Prefix: "wrong", Verifier: "hash", Scopes: []string{"memory:read"}, Generation: 1, CreatedAt: time.Now()}
	if e = s.InsertCredential(ctx, c); !errors.Is(e, store.ErrNotFound) {
		t.Fatalf("cross-account credential: %v", e)
	}
	c.AccountID = a.ID
	c.Prefix = "correct"
	if e = s.InsertCredential(ctx, c); e != nil {
		t.Fatal(e)
	}
	got, e := s.CredentialByPrefix(ctx, "correct")
	if e != nil || got.AccountID != a.ID || got.BrainID != id || got.Generation != 1 {
		t.Fatalf("binding %+v %v", got, e)
	}
}
func TestMigrationAndRollback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.db")
	s, e := store.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	ctx := context.Background()
	e = s.Transaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO audit_log(actor,action,created_at) VALUES('test','rollback','now')`)
		if err != nil {
			return err
		}
		return errors.New("injected failure")
	})
	if e == nil {
		t.Fatal("transaction succeeded")
	}
	var n int
	if e = s.DB().QueryRow(`SELECT count(*) FROM audit_log`).Scan(&n); e != nil || n != 0 {
		t.Fatalf("partial write %d %v", n, e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e = store.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	}()
	if e = s.DB().QueryRow(`SELECT count(*) FROM schema_migrations`).Scan(&n); e != nil || n != 6 {
		t.Fatalf("migration count %d %v", n, e)
	}
}

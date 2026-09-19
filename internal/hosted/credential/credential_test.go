package credential_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func TestRotationAndOwnership(t *testing.T) {
	ctx := context.Background()
	s, e := store.Open(filepath.Join(t.TempDir(), "db"))
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
	id := store.ID()
	if _, e = s.InsertBrain(ctx, a.ID, id, id, "ready", time.Now()); e != nil {
		t.Fatal(e)
	}
	issuer := &credential.Issuer{Store: s}
	raw, e := issuer.Issue(ctx, id, a.ID, []string{"memory:read", "memory:write"})
	if e != nil {
		t.Fatal(e)
	}
	b, e := issuer.Verify(ctx, raw)
	if e != nil || b.BrainID != id {
		t.Fatalf("verify %+v %v", b, e)
	}
	if _, e = issuer.Rotate(ctx, "other", id); !errors.Is(e, store.ErrNotFound) {
		t.Fatalf("ownership %v", e)
	}
	next, e := issuer.Rotate(ctx, a.ID, id)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = issuer.Verify(ctx, raw); !errors.Is(e, credential.ErrRevoked) {
		t.Fatalf("old token %v", e)
	}
	b, e = issuer.Verify(ctx, next)
	if e != nil || b.Generation != 2 {
		t.Fatalf("generation %+v %v", b, e)
	}
	if e = issuer.Revoke(ctx, a.ID, id); e != nil {
		t.Fatal(e)
	}
	if _, e = issuer.Verify(ctx, next); !errors.Is(e, credential.ErrRevoked) {
		t.Fatalf("revoke %v", e)
	}
}

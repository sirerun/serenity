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

func TestPartnerKeysRotateAndRevokeIndependently(t *testing.T) {
	ctx := context.Background()
	s, e := store.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = s.Close() }()
	a, e := s.CreateAccount(ctx, "p@example.com")
	if e != nil {
		t.Fatal(e)
	}
	id := store.ID()
	if _, e = s.InsertBrain(ctx, a.ID, id, id, "ready", time.Now()); e != nil {
		t.Fatal(e)
	}
	if _, e = s.DB().Exec(`INSERT INTO partners(id,display_name,secret_hash,redirect_prefix,status,created_at,updated_at) VALUES('blink','Blink','x','blink://l','active','t','t')`); e != nil {
		t.Fatal(e)
	}
	issuer := &credential.Issuer{Store: s}
	manual, e := issuer.Issue(ctx, id, a.ID, []string{"memory:read"})
	if e != nil {
		t.Fatal(e)
	}
	firstID, first, e := issuer.IssuePartner(ctx, a.ID, id, "blink")
	if e != nil {
		t.Fatal(e)
	}
	if b, e := issuer.Verify(ctx, first); e != nil || b.PartnerID != "blink" || b.CredentialID != firstID || len(b.Scopes) != 2 {
		t.Fatalf("partner binding %+v %v", b, e)
	}
	secondID, second, e := issuer.IssuePartner(ctx, a.ID, id, "blink")
	if e != nil || secondID == firstID {
		t.Fatal(e)
	}
	if _, e = issuer.Verify(ctx, first); !errors.Is(e, credential.ErrRevoked) {
		t.Fatalf("reissue did not rotate: %v", e)
	}
	// The owner replacing a manual token keeps the partner key live.
	replaced, e := issuer.Rotate(ctx, a.ID, id)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = issuer.Verify(ctx, second); e != nil {
		t.Fatalf("manual rotation revoked partner key: %v", e)
	}
	if _, e = issuer.Verify(ctx, manual); !errors.Is(e, credential.ErrRevoked) {
		t.Fatalf("manual rotation %v", e)
	}
	var epochBefore, epochAfter int
	if e = s.DB().QueryRow(`SELECT generation FROM oauth_epochs WHERE brain_id=?`, id).Scan(&epochBefore); e != nil {
		t.Fatal(e)
	}
	if e = issuer.RevokeKey(ctx, secondID); e != nil {
		t.Fatal(e)
	}
	if e = issuer.RevokeKey(ctx, secondID); e != nil {
		t.Fatalf("repeat revoke %v", e)
	}
	if e = issuer.RevokeKey(ctx, "missing"); !errors.Is(e, store.ErrNotFound) {
		t.Fatalf("unknown key %v", e)
	}
	if _, e = issuer.Verify(ctx, second); !errors.Is(e, credential.ErrRevoked) {
		t.Fatalf("revoked key %v", e)
	}
	if _, e = issuer.Verify(ctx, replaced); e != nil {
		t.Fatalf("per-key revoke touched another key: %v", e)
	}
	if e = s.DB().QueryRow(`SELECT generation FROM oauth_epochs WHERE brain_id=?`, id).Scan(&epochAfter); e != nil || epochAfter != epochBefore {
		t.Fatalf("per-key revoke advanced the OAuth epoch %d -> %d %v", epochBefore, epochAfter, e)
	}
}

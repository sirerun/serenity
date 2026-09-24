package oauth

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ajent-social/go/mcpoauth"
	"github.com/sirerun/serenity/internal/hosted/credential"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func TestSQLStoreOneTimeConsumeAndRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "control.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &SQLStore{DB: db}
	now := time.Now().UTC()
	rec := mcpoauth.CodeRecord{ID: store.Hash("synthetic-code"), ExpiresAt: now.Add(time.Minute), CreatedAt: now}
	if err = adapter.CreateCode(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	adapter.DB = db
	var winners atomic.Int32
	var wg sync.WaitGroup
	for range 24 {
		wg.Go(func() {
			if _, e := adapter.ConsumeCode(ctx, rec.ID); e == nil {
				winners.Add(1)
			}
		})
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("redemption winners %d", winners.Load())
	}
}
func TestSQLTokenInsertCannotRaceProjectRevocation(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	a, err := db.CreateAccount(ctx, "synthetic@example.test")
	if err != nil {
		t.Fatal(err)
	}
	brain := store.ID()
	if _, err = db.InsertBrain(ctx, a.ID, brain, brain, "ready", time.Now()); err != nil {
		t.Fatal(err)
	}
	adapter := &SQLStore{DB: db}
	now := time.Now().UTC()
	raw := strings.Repeat("a", 43)
	record := mcpoauth.TokenRecord{GrantID: "test-grant", ID: store.Hash(raw), ClientID: "synthetic", Subject: a.ID, Binding: brain + ".0", Resource: "https://memory.example/mcp", Scopes: []string{"memory:read"}, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	issuer := credential.Issuer{Store: db}
	if err = issuer.Revoke(ctx, a.ID, brain); err != nil {
		t.Fatal(err)
	}
	if err = adapter.CreateGrant(ctx, grantIssue(record)); err == nil {
		t.Fatal("old authorization exchanged after project revocation")
	}
	record.Binding = brain + ".1"
	if err = adapter.CreateGrant(ctx, grantIssue(record)); err != nil {
		t.Fatal(err)
	}
	h, err := New(db, nil, nil, "https://memory.example", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.Verify(ctx, raw); err != nil {
		t.Fatalf("new authorization rejected: %v", err)
	}
	if _, err = db.DB().ExecContext(ctx, "UPDATE accounts SET status='deleting' WHERE id=?", a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = h.Verify(ctx, raw); err == nil {
		t.Fatal("disabled account accepted")
	}
}

func grantIssue(t mcpoauth.TokenRecord) mcpoauth.GrantIssue {
	return mcpoauth.GrantIssue{Grant: mcpoauth.GrantRecord{ID: t.GrantID, ClientID: t.ClientID, Subject: t.Subject, Binding: t.Binding, Resource: t.Resource, Scopes: t.Scopes, CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt}, Token: t}
}

func TestSQLConcurrentRefreshCommitsFamilyRevocation(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "control.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	other, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := other.Close(); err != nil {
			t.Error(err)
		}
	}()
	a, err := db.CreateAccount(ctx, "refresh@example.test")
	if err != nil {
		t.Fatal(err)
	}
	brain := store.ID()
	if _, err = db.InsertBrain(ctx, a.ID, brain, brain, "ready", time.Now()); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	grant := mcpoauth.GrantRecord{ID: "family", ClientID: "client", Subject: a.ID, Binding: brain + ".0", Resource: "https://memory.example/mcp", Scopes: []string{"memory:read"}, CreatedAt: now, ExpiresAt: now.Add(24 * time.Hour)}
	token := mcpoauth.TokenRecord{ID: "initial-access", GrantID: grant.ID, ClientID: grant.ClientID, Subject: grant.Subject, Binding: grant.Binding, Resource: grant.Resource, Scopes: grant.Scopes, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	refresh := mcpoauth.RefreshRecord{ID: "initial-refresh", GrantID: grant.ID, ClientID: grant.ClientID, Generation: 1, CreatedAt: now, ExpiresAt: grant.ExpiresAt}
	adapters := []*SQLStore{{DB: db}, {DB: other}}
	if err = adapters[0].CreateGrant(ctx, mcpoauth.GrantIssue{Grant: grant, Token: token, Refresh: &refresh}); err != nil {
		t.Fatal(err)
	}
	var won, reused atomic.Int32
	var wg sync.WaitGroup
	for i, adapter := range adapters {
		wg.Go(func() {
			nextToken := token
			nextToken.ID = fmt.Sprintf("access-%d", i)
			next := refresh
			next.ID = fmt.Sprintf("refresh-%d", i)
			next.Generation = 2
			err := adapter.RotateRefresh(ctx, mcpoauth.RefreshRotation{RefreshID: refresh.ID, At: now, Token: nextToken, Refresh: next})
			if err == nil {
				won.Add(1)
			} else if errors.Is(err, mcpoauth.ErrReused) {
				reused.Add(1)
			} else {
				t.Errorf("rotation: %v", err)
			}
		})
	}
	wg.Wait()
	if won.Load() != 1 || reused.Load() != 1 {
		t.Fatalf("won=%d reused=%d", won.Load(), reused.Load())
	}
	got, err := adapters[1].Grant(ctx, grant.ID)
	if err != nil || got.RevokedAt.IsZero() {
		t.Fatal("reuse error rolled back family revocation")
	}
}

package oauth

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ajent-social/go/mcpoauth"
	"github.com/sirerun/serenity/internal/hosted/store"
)

// lifecycleStore opens a fresh control database with one active account and
// one ready project so grants pass live policy.
func lifecycleStore(t *testing.T) (*SQLStore, store.Account, string) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	a, err := db.CreateAccount(ctxOf(t), t.Name()+"@example.test")
	if err != nil {
		t.Fatal(err)
	}
	brain := store.ID()
	if _, err = db.InsertBrain(ctxOf(t), a.ID, brain, brain, "ready", time.Now()); err != nil {
		t.Fatal(err)
	}
	return &SQLStore{DB: db}, a, brain
}

func ctxOf(t *testing.T) context.Context { t.Helper(); return context.Background() }

func clientExpiry(t *testing.T, db *sql.DB, id string) time.Time {
	t.Helper()
	var raw string
	if err := db.QueryRow("SELECT expires_at FROM oauth_clients WHERE id=?", id).Scan(&raw); err != nil {
		t.Fatalf("client %s expiry: %v", id, err)
	}
	ts, err := time.Parse("2006-01-02T15:04:05.000000000Z", raw)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

func count(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func family(subject, binding, client, tag string, now time.Time) mcpoauth.GrantIssue {
	grant := mcpoauth.GrantRecord{ID: "grant-" + tag, ClientID: client, Subject: subject, Binding: binding, Resource: "https://memory.example/mcp", Scopes: []string{"memory:read"}, CreatedAt: now, ExpiresAt: now.Add(30 * 24 * time.Hour)}
	token := mcpoauth.TokenRecord{ID: "token-" + tag, GrantID: grant.ID, ClientID: client, Subject: subject, Binding: binding, Resource: grant.Resource, Scopes: grant.Scopes, CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	refresh := &mcpoauth.RefreshRecord{ID: "refresh-" + tag, GrantID: grant.ID, ClientID: client, Generation: 1, CreatedAt: now, ExpiresAt: grant.ExpiresAt}
	return mcpoauth.GrantIssue{Grant: grant, Token: token, Refresh: refresh}
}

// A client registration is retained for one hour by itself, but a pending
// consent, authorization code or issued grant must keep its callback alive
// for as long as that record can still be redeemed, and never shorten it.
func TestClientRetentionFollowsPendingConsentCodeAndGrant(t *testing.T) {
	ctx := context.Background()
	s, a, brain := lifecycleStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	client := mcpoauth.Client{ID: "client-retained", Name: "test", RedirectURIs: []string{"https://app.example/cb"}, CreatedAt: now.Add(-50 * time.Minute)}
	if err := s.CreateClient(ctx, client); err != nil {
		t.Fatal(err)
	}
	base := clientExpiry(t, s.DB.DB(), client.ID)
	if !base.Equal(client.CreatedAt.Add(time.Hour)) {
		t.Fatalf("base retention %v, want %v", base, client.CreatedAt.Add(time.Hour))
	}

	consent := mcpoauth.ConsentRecord{ID: store.Hash("consent"), ClientID: client.ID, RedirectURI: client.RedirectURIs[0], Resource: "https://memory.example/mcp", CreatedAt: now, ExpiresAt: now.Add(45 * time.Minute)}
	if err := s.CreateConsent(ctx, consent); err != nil {
		t.Fatal(err)
	}
	if got, want := clientExpiry(t, s.DB.DB(), client.ID), consent.ExpiresAt.Add(10*time.Minute); !got.Equal(want) {
		t.Fatalf("after consent client expires %v, want %v", got, want)
	}

	// A code that is redeemable for less time than the consent must not pull
	// the retention back in.
	code := mcpoauth.CodeRecord{ID: store.Hash("code"), ClientID: client.ID, Subject: a.ID, Binding: brain + ".0", CreatedAt: now, ExpiresAt: now.Add(5 * time.Minute)}
	if err := s.CreateCode(ctx, code); err != nil {
		t.Fatal(err)
	}
	if got, want := clientExpiry(t, s.DB.DB(), client.ID), consent.ExpiresAt.Add(10*time.Minute); !got.Equal(want) {
		t.Fatalf("shorter code shortened client retention to %v, want %v", got, want)
	}

	// A code that outlives the consent extends again.
	longCode := code
	longCode.ID = store.Hash("code-long")
	longCode.ExpiresAt = now.Add(2 * time.Hour)
	if err := s.CreateCode(ctx, longCode); err != nil {
		t.Fatal(err)
	}
	if got, want := clientExpiry(t, s.DB.DB(), client.ID), longCode.ExpiresAt.Add(10*time.Minute); !got.Equal(want) {
		t.Fatalf("after code client expires %v, want %v", got, want)
	}

	// An issued grant keeps the registration a week past the grant.
	issue := family(a.ID, brain+".0", client.ID, "retained", now)
	if err := s.CreateGrant(ctx, issue); err != nil {
		t.Fatal(err)
	}
	if got, want := clientExpiry(t, s.DB.DB(), client.ID), issue.Grant.ExpiresAt.Add(7*24*time.Hour); !got.Equal(want) {
		t.Fatalf("after grant client expires %v, want %v", got, want)
	}

	// A consent or code that names an unknown client must not create one.
	orphan := consent
	orphan.ID = store.Hash("orphan")
	orphan.ClientID = "client-unknown"
	if err := s.CreateConsent(ctx, orphan); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Client(ctx, orphan.ClientID); !errors.Is(err, mcpoauth.ErrNotFound) {
		t.Fatalf("consent for unknown client materialised a registration: %v", err)
	}
}

// seedClients fills oauth_clients to capacity in one statement. Row n gets
// expires_at = base + n seconds, so lower n is evicted first.
func seedClients(t *testing.T, db *sql.DB, n int, base time.Time) {
	t.Helper()
	_, err := db.Exec(`WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n<?)
INSERT INTO oauth_clients(id,record,expires_at)
SELECT 'bulk-'||n, json_object('id','bulk-'||n,'name','bulk','redirect_uris',json_array('https://app.example/cb'),'created_at',?), strftime('%Y-%m-%dT%H:%M:%f000Z', ?, '+'||n||' seconds') FROM seq`, n, store.Stamp(base), store.Stamp(base))
	if err != nil {
		t.Fatal(err)
	}
	if got := count(t, db, "oauth_clients"); got != n {
		t.Fatalf("seeded %d clients, want %d", got, n)
	}
}

func TestClientCapacityEvictsOnlyUnreferencedRegistrations(t *testing.T) {
	ctx := context.Background()
	s, a, brain := lifecycleStore(t)
	db := s.DB.DB()
	now := time.Now().UTC()
	// Every seeded registration is still within retention, so only capacity
	// eviction can remove any of them.
	seedClients(t, db, 10000, now.Add(time.Hour))

	// Protect three of the registrations that sort first for eviction.
	byGrant, byConsent, byCode, staleRef := "bulk-1", "bulk-2", "bulk-3", "bulk-4"
	if err := s.CreateGrant(ctx, family(a.ID, brain+".0", byGrant, "protect", now)); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateConsent(ctx, mcpoauth.ConsentRecord{ID: store.Hash("pending-consent"), ClientID: byConsent, CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateCode(ctx, mcpoauth.CodeRecord{ID: store.Hash("pending-code"), ClientID: byCode, Subject: a.ID, Binding: brain + ".0", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	// An already-expired consent no longer protects its client.
	if _, err := db.Exec("INSERT INTO oauth_consents(id,record,expires_at) VALUES(?,?,?)", store.Hash("stale-consent"), `{"id":"stale","client_id":"`+staleRef+`"}`, store.Stamp(now.Add(-time.Minute))); err != nil {
		t.Fatal(err)
	}
	// Referencing records extended these registrations; force them back to
	// the front of the eviction order so protection, not ordering, is tested.
	if _, err := db.Exec("UPDATE oauth_clients SET expires_at=? WHERE id IN (?,?,?)", store.Stamp(now.Add(time.Minute)), byGrant, byConsent, byCode); err != nil {
		t.Fatal(err)
	}

	fresh := mcpoauth.Client{ID: "client-fresh", Name: "fresh", RedirectURIs: []string{"https://app.example/cb"}, CreatedAt: now}
	if err := s.CreateClient(ctx, fresh); err != nil {
		t.Fatalf("registration at capacity with evictable rows: %v", err)
	}
	if _, err := s.Client(ctx, fresh.ID); err != nil {
		t.Fatalf("fresh client not stored: %v", err)
	}
	for _, id := range []string{byGrant, byConsent, byCode} {
		if _, err := s.Client(ctx, id); err != nil {
			t.Errorf("referenced client %s evicted: %v", id, err)
		}
	}
	if _, err := s.Client(ctx, staleRef); !errors.Is(err, mcpoauth.ErrNotFound) {
		t.Errorf("client referenced only by an expired consent survived: %v", err)
	}
	// One eviction batch of 100 unreferenced rows, then the new row.
	if got := count(t, db, "oauth_clients"); got != 10000-100+1 {
		t.Fatalf("client rows %d, want %d", got, 10000-100+1)
	}
	// Eviction took the oldest unreferenced rows: bulk-4..bulk-103 gone, bulk-104 kept.
	if _, err := s.Client(ctx, "bulk-103"); !errors.Is(err, mcpoauth.ErrNotFound) {
		t.Errorf("bulk-103 should be within the evicted window: %v", err)
	}
	if _, err := s.Client(ctx, "bulk-104"); err != nil {
		t.Errorf("bulk-104 should survive the eviction window: %v", err)
	}
	// The grant family that protected byGrant is intact.
	if _, err := s.Token(ctx, "token-protect"); err != nil {
		t.Errorf("token lost during eviction: %v", err)
	}
}

func TestClientCapacityRefusesWhenEveryRegistrationIsReferenced(t *testing.T) {
	if testing.Short() {
		// The eviction scan is O(clients x live consents) because the
		// json_extract comparison cannot use the expression index; this case
		// takes ~20s at full capacity.
		t.Skip("full-capacity eviction scan is slow")
	}
	ctx := context.Background()
	s, _, _ := lifecycleStore(t)
	db := s.DB.DB()
	now := time.Now().UTC()
	seedClients(t, db, 10000, now.Add(time.Hour))
	// One pending consent per client, all still redeemable.
	if _, err := db.Exec(`WITH RECURSIVE seq(n) AS (SELECT 1 UNION ALL SELECT n+1 FROM seq WHERE n<10000)
INSERT INTO oauth_consents(id,record,expires_at) SELECT 'consent-'||n, json_object('id','consent-'||n,'client_id','bulk-'||n), ? FROM seq`, store.Stamp(now.Add(10*time.Minute))); err != nil {
		t.Fatal(err)
	}
	err := s.CreateClient(ctx, mcpoauth.Client{ID: "client-overflow", CreatedAt: now})
	if err == nil || !strings.Contains(err.Error(), "capacity") {
		t.Fatalf("expected capacity error, got %v", err)
	}
	if got := count(t, db, "oauth_clients"); got != 10000 {
		t.Fatalf("refused registration still removed rows: %d", got)
	}
	if _, err = s.Client(ctx, "client-overflow"); !errors.Is(err, mcpoauth.ErrNotFound) {
		t.Fatalf("refused registration was stored: %v", err)
	}
}

func TestReauthorizationReplacesPriorFamilyForSameClientAndProject(t *testing.T) {
	ctx := context.Background()
	s, a, brain := lifecycleStore(t)
	now := time.Now().UTC()
	binding := brain + ".0"
	first := family(a.ID, binding, "client-a", "first", now)
	if err := s.CreateGrant(ctx, first); err != nil {
		t.Fatal(err)
	}
	// The same project through a different client, and the same client on a
	// different project, must both survive the replacement.
	otherClient := family(a.ID, binding, "client-b", "other-client", now)
	if err := s.CreateGrant(ctx, otherClient); err != nil {
		t.Fatal(err)
	}
	// Accounts own one project each, so the other project needs its own account.
	b, err := s.DB.CreateAccount(ctx, "second-"+t.Name()+"@example.test")
	if err != nil {
		t.Fatal(err)
	}
	otherBrain := store.ID()
	if _, err = s.DB.InsertBrain(ctx, b.ID, otherBrain, otherBrain, "ready", time.Now()); err != nil {
		t.Fatal(err)
	}
	otherProject := family(b.ID, otherBrain+".0", "client-a", "other-project", now)
	if err := s.CreateGrant(ctx, otherProject); err != nil {
		t.Fatal(err)
	}

	second := family(a.ID, binding, "client-a", "second", now)
	if err := s.CreateGrant(ctx, second); err != nil {
		t.Fatalf("re-authorization rejected: %v", err)
	}
	for name, err := range map[string]error{
		"old grant":   must(s.Grant(ctx, first.Grant.ID)),
		"old token":   must(s.Token(ctx, first.Token.ID)),
		"old refresh": must(s.Refresh(ctx, first.Refresh.ID)),
	} {
		if !errors.Is(err, mcpoauth.ErrNotFound) {
			t.Errorf("%s survived re-authorization: %v", name, err)
		}
	}
	for _, keep := range []mcpoauth.GrantIssue{second, otherClient, otherProject} {
		if _, err := s.Grant(ctx, keep.Grant.ID); err != nil {
			t.Errorf("grant %s: %v", keep.Grant.ID, err)
		}
		if _, err := s.Token(ctx, keep.Token.ID); err != nil {
			t.Errorf("token %s: %v", keep.Token.ID, err)
		}
		if _, err := s.Refresh(ctx, keep.Refresh.ID); err != nil {
			t.Errorf("refresh %s: %v", keep.Refresh.ID, err)
		}
	}
	if got := count(t, s.DB.DB(), "oauth_tokens"); got != 3 {
		t.Fatalf("token rows %d, want 3", got)
	}
	// The old refresh token cannot be rotated into the new family.
	rot := mcpoauth.RefreshRotation{RefreshID: first.Refresh.ID, At: now, Token: second.Token, Refresh: *second.Refresh}
	if err := s.RotateRefresh(ctx, rot); !errors.Is(err, mcpoauth.ErrNotFound) {
		t.Fatalf("replaced refresh token rotated: %v", err)
	}
}

func TestFailedReauthorizationKeepsPriorFamily(t *testing.T) {
	ctx := context.Background()
	s, a, brain := lifecycleStore(t)
	now := time.Now().UTC()
	binding := brain + ".0"
	prior := family(a.ID, binding, "client-a", "prior", now)
	if err := s.CreateGrant(ctx, prior); err != nil {
		t.Fatal(err)
	}
	// Another family owns this access-token hash so a later insert conflicts.
	other := family(a.ID, binding, "client-b", "other", now)
	if err := s.CreateGrant(ctx, other); err != nil {
		t.Fatal(err)
	}

	cases := map[string]mcpoauth.GrantIssue{}
	mismatched := family(a.ID, binding, "client-a", "mismatch", now)
	mismatched.Token.GrantID = "someone-else"
	cases["token bound to a different grant"] = mismatched
	dupToken := family(a.ID, binding, "client-a", "dup-token", now)
	dupToken.Token.ID = other.Token.ID
	cases["access token hash already stored"] = dupToken
	dupRefresh := family(a.ID, binding, "client-a", "dup-refresh", now)
	dupRefresh.Refresh.ID = other.Refresh.ID
	cases["refresh token hash already stored"] = dupRefresh

	for name, issue := range cases {
		t.Run(name, func(t *testing.T) {
			if err := s.CreateGrant(ctx, issue); err == nil {
				t.Fatal("invalid re-authorization accepted")
			}
			if _, err := s.Grant(ctx, prior.Grant.ID); err != nil {
				t.Fatalf("prior grant lost: %v", err)
			}
			if _, err := s.Token(ctx, prior.Token.ID); err != nil {
				t.Fatalf("prior token lost: %v", err)
			}
			if _, err := s.Refresh(ctx, prior.Refresh.ID); err != nil {
				t.Fatalf("prior refresh lost: %v", err)
			}
			if _, err := s.Grant(ctx, issue.Grant.ID); !errors.Is(err, mcpoauth.ErrNotFound) {
				t.Fatalf("partial family committed: %v", err)
			}
			if got := count(t, s.DB.DB(), "oauth_grants"); got != 2 {
				t.Fatalf("grant rows %d, want 2", got)
			}
		})
	}
	// The prior family still works end to end after every failed attempt.
	next := *prior.Refresh
	next.ID, next.Generation = "refresh-prior-2", 2
	nextToken := prior.Token
	nextToken.ID = "token-prior-2"
	if err := s.RotateRefresh(ctx, mcpoauth.RefreshRotation{RefreshID: prior.Refresh.ID, At: now, Token: nextToken, Refresh: next}); err != nil {
		t.Fatalf("prior family unusable after failed replacement: %v", err)
	}
}

func must[T any](_ T, err error) error { return err }

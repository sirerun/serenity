package identity_test

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/identity"
	"github.com/sirerun/serenity/internal/hosted/store"
)

type sender struct{ link string }

func (s *sender) Send(_ context.Context, _, link string) error { s.link = link; return nil }
func TestSingleUseAndSession(t *testing.T) {
	ctx := context.Background()
	db, e := store.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	mail := &sender{}
	now := time.Now()
	s := &identity.Service{Store: db, Sender: mail, Origin: "https://example.test", Clock: func() time.Time { return now }}
	if e = s.RequestLink(ctx, "person@example.com", "127.0.0.1"); e != nil {
		t.Fatal(e)
	}
	u, e := url.Parse(mail.link)
	if e != nil {
		t.Fatal(e)
	}
	token := u.Query().Get("token")
	var wg sync.WaitGroup
	results := make(chan string, 2)
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); v, e := s.Consume(ctx, token); results <- v; errs <- e }()
	}
	wg.Wait()
	close(results)
	close(errs)
	passed := 0
	for e := range errs {
		if e == nil {
			passed++
		} else if !errors.Is(e, identity.ErrExpiredOrUsed) {
			t.Fatal(e)
		}
	}
	if passed != 1 {
		t.Fatalf("single-use: %d successes", passed)
	}
	var raw string
	for v := range results {
		if v != "" {
			raw = v
		}
	}
	session, e := s.Session(ctx, raw)
	if e != nil {
		t.Fatal(e)
	}
	if identity.CheckCSRF(session, "") || identity.CheckCSRF(session, "wrong") || !identity.CheckCSRF(session, session.CSRF) {
		t.Fatal("CSRF failure")
	}
	if _, e = db.DB().ExecContext(ctx, `UPDATE accounts SET status='deleting' WHERE id=?`, session.AccountID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Session(ctx, raw); !errors.Is(e, store.ErrNotFound) {
		t.Fatalf("deleting account admitted: %v", e)
	}
	if _, e = s.DeletionSession(ctx, raw); e != nil {
		t.Fatalf("deletion retry blocked: %v", e)
	}
	if _, e = db.DB().ExecContext(ctx, `UPDATE accounts SET status='active' WHERE id=?`, session.AccountID); e != nil {
		t.Fatal(e)
	}
	if e = s.Logout(ctx, raw); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Session(ctx, raw); !errors.Is(e, store.ErrNotFound) {
		t.Fatalf("logout: %v", e)
	}
	if e = s.RequestLink(ctx, "person@example.com", "127.0.0.1"); e != nil {
		t.Fatal(e)
	}
	u, _ = url.Parse(mail.link)
	now = now.Add(16 * time.Minute)
	if _, e = s.Consume(ctx, u.Query().Get("token")); !errors.Is(e, identity.ErrExpiredOrUsed) {
		t.Fatalf("expiry: %v", e)
	}
}
func TestRateLimit(t *testing.T) {
	ctx := context.Background()
	db, e := store.Open(filepath.Join(t.TempDir(), "db"))
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	s := &identity.Service{Store: db, Sender: &sender{}, Origin: "https://example.test"}
	for i := 0; i < 6; i++ {
		e = s.RequestLink(ctx, "person@example.com", "127.0.0.1")
		if i < 5 && e != nil {
			t.Fatal(e)
		}
		if i == 5 && !errors.Is(e, identity.ErrRateLimited) {
			t.Fatalf("sixth request: %v", e)
		}
	}
}

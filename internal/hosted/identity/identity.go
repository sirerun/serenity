// Package identity implements email-link identity independently of MCP credentials.
package identity

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/mail"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/sirerun/serenity/internal/hosted/store"
)

var ErrExpiredOrUsed = errors.New("link expired or already used")
var ErrRateLimited = errors.New("too many login requests")
var ErrInvalidEmail = errors.New("invalid email address")
var ErrCapacity = errors.New("registration capacity reached")

type Sender interface {
	Send(context.Context, string, string) error
}
type Service struct {
	Store      *store.Store
	Sender     Sender
	Origin     string
	Clock      func() time.Time
	AccountCap int
	mu         sync.Mutex
	attempts   map[string][]time.Time
}
type Session struct{ ID, AccountID, CSRF string }

func (s *Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}
func token() string {
	v := make([]byte, 32)
	rand.Read(v)
	return base64.RawURLEncoding.EncodeToString(v)
}
func (s *Service) admit(email, ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attempts == nil {
		s.attempts = map[string][]time.Time{}
	}
	now := s.now()
	cut := now.Add(-time.Minute)
	for k, values := range s.attempts {
		n := 0
		for _, t := range values {
			if t.After(cut) {
				values[n] = t
				n++
			}
		}
		if n == 0 {
			delete(s.attempts, k)
		} else {
			s.attempts[k] = values[:n]
		}
	}
	keys := []string{"email:" + store.Hash(email), "ip:" + ip}
	for _, k := range keys {
		if len(s.attempts[k]) >= 5 {
			return false
		}
	}
	if len(s.attempts) > 10000 {
		return false
	}
	for _, k := range keys {
		s.attempts[k] = append(s.attempts[k], now)
	}
	return true
}
func (s *Service) RequestLink(ctx context.Context, email, ip string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 254 {
		return ErrInvalidEmail
	}
	if !s.admit(email, ip) {
		return ErrRateLimited
	}
	raw := token()
	now := s.now()
	err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `INSERT INTO login_tokens(id,email,token_hash,created_at,expires_at) VALUES(?,?,?,?,?)`, store.ID(), email, store.Hash(raw), store.Stamp(now), store.Stamp(now.Add(15*time.Minute)))
		return e
	})
	if err != nil {
		return err
	}
	if err = s.Sender.Send(ctx, email, s.Origin+"/login/consume?token="+url.QueryEscape(raw)); err != nil {
		cleanup := s.Store.Transaction(ctx, func(tx *sql.Tx) error {
			_, e := tx.ExecContext(ctx, `DELETE FROM login_tokens WHERE token_hash=?`, store.Hash(raw))
			return e
		})
		return errors.Join(errors.New("send login email failed"), cleanup)
	}
	return nil
}

// Consume creates the account and session atomically with consuming the link.
func (s *Service) Consume(ctx context.Context, raw string) (sessionToken string, err error) {
	if len(raw) != 43 {
		return "", ErrExpiredOrUsed
	}
	now := s.now()
	err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var email, expires string
		var consumed sql.NullString
		e := tx.QueryRowContext(ctx, `SELECT email,expires_at,consumed_at FROM login_tokens WHERE token_hash=?`, store.Hash(raw)).Scan(&email, &expires, &consumed)
		if errors.Is(e, sql.ErrNoRows) {
			return ErrExpiredOrUsed
		}
		if e != nil {
			return e
		}
		expiry, e := time.Parse(time.RFC3339Nano, expires)
		if e != nil {
			return e
		}
		if consumed.Valid || !expiry.After(now) {
			return ErrExpiredOrUsed
		}
		var accountID string
		e = tx.QueryRowContext(ctx, `SELECT id FROM accounts WHERE email_hash=? AND status='active'`, store.Hash(email)).Scan(&accountID)
		if errors.Is(e, sql.ErrNoRows) {
			var count int
			if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM accounts WHERE status='active'`).Scan(&count); e != nil {
				return e
			}
			cap := s.AccountCap
			if cap <= 0 {
				cap = 100
			}
			if count >= cap {
				return ErrCapacity
			}
			accountID = store.ID()
			_, e = tx.ExecContext(ctx, `INSERT INTO accounts(id,email_hash,email,created_at,status,plan_id,plan_version) VALUES(?,?,?,?,'active','free',1)`, accountID, store.Hash(email), email, store.Stamp(now))
		}
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `UPDATE login_tokens SET consumed_at=? WHERE token_hash=?`, store.Stamp(now), store.Hash(raw))
		if e != nil {
			return e
		}
		sessionToken = token()
		_, e = tx.ExecContext(ctx, `INSERT INTO sessions(id,account_id,token_hash,created_at,expires_at,csrf_secret) VALUES(?,?,?,?,?,?)`, store.ID(), accountID, store.Hash(sessionToken), store.Stamp(now), store.Stamp(now.Add(30*24*time.Hour)), token())
		return e
	})
	if err != nil {
		sessionToken = ""
	}
	return
}
func (s *Service) Session(ctx context.Context, raw string) (session Session, err error) {
	if len(raw) != 43 {
		return session, store.ErrNotFound
	}
	err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var expires string
		e := tx.QueryRowContext(ctx, `SELECT s.id,s.account_id,s.csrf_secret,s.expires_at FROM sessions s JOIN accounts a ON a.id=s.account_id WHERE s.token_hash=? AND a.status='active'`, store.Hash(raw)).Scan(&session.ID, &session.AccountID, &session.CSRF, &expires)
		if errors.Is(e, sql.ErrNoRows) {
			return store.ErrNotFound
		}
		if e != nil {
			return e
		}
		expiry, e := time.Parse(time.RFC3339Nano, expires)
		if e != nil {
			return e
		}
		if !expiry.After(s.now()) {
			return store.ErrNotFound
		}
		_, e = tx.ExecContext(ctx, `UPDATE sessions SET expires_at=? WHERE id=?`, store.Stamp(s.now().Add(30*24*time.Hour)), session.ID)
		return e
	})
	return
}
func (s *Service) Logout(ctx context.Context, raw string) error {
	return s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash=?`, store.Hash(raw))
		return e
	})
}
func CheckCSRF(session Session, submitted string) bool {
	return submitted != "" && subtle.ConstantTimeCompare([]byte(session.CSRF), []byte(submitted)) == 1
}

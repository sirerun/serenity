package identity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/sirerun/serenity/internal/hosted/store"
)

var ErrWorkOS = errors.New("unable to complete external sign-in")
var ErrLinkRequired = errors.New("sign in with your email link, then connect this sign-in method in settings")

// WorkOS exchanges codes only with the official AuthKit API. Secrets and tokens
// never enter browser URLs, logs or the control database. Serenity owns sessions.
type WorkOS struct {
	ClientID   string
	APIKey     string
	HTTPClient *http.Client
}
type workOSUser struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
}

func (w *WorkOS) exchange(ctx context.Context, code, verifier string) (workOSUser, error) {
	var result struct {
		User workOSUser `json:"user"`
	}
	payload, err := json.Marshal(map[string]string{"client_id": w.ClientID, "client_secret": w.APIKey, "grant_type": "authorization_code", "code": code, "code_verifier": verifier})
	if err != nil {
		return result.User, ErrWorkOS
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.workos.com/user_management/authenticate", bytes.NewReader(payload))
	if err != nil {
		return result.User, ErrWorkOS
	}
	req.Header.Set("Content-Type", "application/json")
	client := http.Client{Timeout: 15 * time.Second}
	if w.HTTPClient != nil {
		client = *w.HTTPClient
	}
	// Never forward the code or client secret across an API redirect.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	resp, err := client.Do(req.WithContext(ctx))
	if err != nil {
		return result.User, ErrWorkOS
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return result.User, ErrWorkOS
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20+1))
	if err != nil || len(data) > 1<<20 || json.Unmarshal(data, &result) != nil || result.User.ID == "" || len(result.User.ID) > 256 || !result.User.EmailVerified {
		return workOSUser{}, ErrWorkOS
	}
	return result.User, nil
}

// BeginWorkOS binds the single-use state to this browser. linkSession is only
// supplied by the dashboard after CSRF-checked, explicit account-link consent.
func (s *Service) BeginWorkOS(ctx context.Context, linkSession, ip string) (state, location string, err error) {
	if s.WorkOS == nil {
		return "", "", ErrWorkOS
	}
	if !s.admit("workos-start:"+ip, ip) {
		return "", "", ErrRateLimited
	}
	var accountID any
	if linkSession != "" {
		session, e := s.Session(ctx, linkSession)
		if e != nil {
			return "", "", ErrWorkOS
		}
		accountID = session.AccountID
	}
	state, verifier := token(), token()
	sessionHash := ""
	if linkSession != "" {
		sessionHash = store.Hash(linkSession)
	}
	now := s.now()
	err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if _, e := tx.ExecContext(ctx, `DELETE FROM workos_logins WHERE expires_at<=?`, store.Stamp(now)); e != nil {
			return e
		}
		var pending int
		if e := tx.QueryRowContext(ctx, `SELECT count(*) FROM workos_logins`).Scan(&pending); e != nil {
			return e
		}
		if pending >= 10000 {
			return ErrRateLimited
		}
		_, e := tx.ExecContext(ctx, `INSERT INTO workos_logins(state_hash,verifier,session_hash,account_id,client_id,expires_at) VALUES(?,?,?,?,?,?)`, store.Hash(state), verifier, sessionHash, accountID, s.WorkOS.ClientID, store.Stamp(now.Add(10*time.Minute)))
		return e
	})
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(verifier))
	q := url.Values{"client_id": {s.WorkOS.ClientID}, "provider": {"authkit"}, "response_type": {"code"}, "redirect_uri": {s.Origin + "/login/workos/callback"}, "state": {state}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}}
	return state, "https://api.workos.com/user_management/authorize?" + q.Encode(), nil
}

// CompleteWorkOS consumes browser-bound state before contacting WorkOS, including
// denial/error callbacks. A failed exchange requires a fresh login attempt.
func (s *Service) CompleteWorkOS(ctx context.Context, state, browserState, code, sessionRaw string) (raw string, err error) {
	if s.WorkOS == nil || len(state) != 43 || subtle.ConstantTimeCompare([]byte(state), []byte(browserState)) != 1 {
		return "", ErrWorkOS
	}
	var verifier, sessionHash, clientID, expiry string
	err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		e := tx.QueryRowContext(ctx, `SELECT verifier,session_hash,client_id,expires_at FROM workos_logins WHERE state_hash=?`, store.Hash(state)).Scan(&verifier, &sessionHash, &clientID, &expiry)
		if e != nil {
			return ErrWorkOS
		}
		_, e = tx.ExecContext(ctx, `DELETE FROM workos_logins WHERE state_hash=?`, store.Hash(state))
		return e
	})
	if err != nil {
		return "", err
	}
	expires, e := time.Parse(time.RFC3339Nano, expiry)
	if e != nil || !expires.After(s.now()) || clientID != s.WorkOS.ClientID || code == "" || len(code) > 4096 {
		return "", ErrWorkOS
	}
	if sessionHash != "" && (sessionRaw == "" || subtle.ConstantTimeCompare([]byte(sessionHash), []byte(store.Hash(sessionRaw))) != 1) {
		return "", ErrWorkOS
	}
	user, err := s.WorkOS.exchange(ctx, code, verifier)
	if err != nil {
		return "", err
	}
	email, err := NormalizeEmail(user.Email)
	if err != nil {
		return "", ErrWorkOS
	}
	allowed, err := s.registrationAllows(ctx, email)
	if err != nil {
		return "", err
	}
	now := s.now()
	err = s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var accountID, status string
		e := tx.QueryRowContext(ctx, `SELECT a.id,a.status FROM workos_identities w JOIN accounts a ON a.id=w.account_id WHERE w.client_id=? AND w.subject=?`, clientID, user.ID).Scan(&accountID, &status)
		if e == nil {
			if status != "active" {
				return ErrAccountUnavailable
			}
			if sessionHash != "" {
				if e = validLinkSession(ctx, tx, sessionHash, accountID, now); e != nil {
					return e
				}
			}
		} else if errors.Is(e, sql.ErrNoRows) {
			e = tx.QueryRowContext(ctx, `SELECT id,status FROM accounts WHERE email_hash=?`, store.Hash(email)).Scan(&accountID, &status)
			if e == nil {
				if status != "active" {
					return ErrAccountUnavailable
				}
				if sessionHash == "" {
					return ErrLinkRequired
				}
				if e = validLinkSession(ctx, tx, sessionHash, accountID, now); e != nil {
					return e
				}
			} else if errors.Is(e, sql.ErrNoRows) {
				// A linking request must never create or switch accounts.
				if sessionHash != "" {
					return ErrLinkRequired
				}
				if !allowed {
					return ErrInviteRequired
				}
				var count int
				if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM accounts WHERE status='active' AND created_by_partner IS NULL`).Scan(&count); e != nil {
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
				if _, e = tx.ExecContext(ctx, `INSERT INTO accounts(id,email_hash,email,created_at,status,plan_id,plan_version) VALUES(?,?,?,?,'active','free',1)`, accountID, store.Hash(email), email, store.Stamp(now)); e != nil {
					return e
				}
			} else {
				return e
			}

			// Only a verified same-email, explicitly session-bound request can reach
			// an existing account here. Replacing its former subject also revokes
			// every browser session; MCP grants are separately authorized and retained.
			var priorSubject string
			e = tx.QueryRowContext(ctx, `SELECT subject FROM workos_identities WHERE client_id=? AND account_id=?`, clientID, accountID).Scan(&priorSubject)
			if e != nil && !errors.Is(e, sql.ErrNoRows) {
				return e
			}
			if e == nil && priorSubject != user.ID {
				if _, e = tx.ExecContext(ctx, `DELETE FROM sessions WHERE account_id=?`, accountID); e != nil {
					return e
				}
				if _, e = tx.ExecContext(ctx, `DELETE FROM workos_logins WHERE account_id=?`, accountID); e != nil {
					return e
				}
			}
			if _, e = tx.ExecContext(ctx, `INSERT INTO workos_identities(client_id,subject,account_id,created_at) VALUES(?,?,?,?) ON CONFLICT(client_id,account_id) DO UPDATE SET subject=excluded.subject,created_at=excluded.created_at`, clientID, user.ID, accountID, store.Stamp(now)); e != nil {

				return ErrLinkRequired
			}
		} else {
			return e
		}
		raw = token()
		_, e = tx.ExecContext(ctx, `INSERT INTO sessions(id,account_id,token_hash,created_at,expires_at,csrf_secret) VALUES(?,?,?,?,?,?)`, store.ID(), accountID, store.Hash(raw), store.Stamp(now), store.Stamp(now.Add(30*24*time.Hour)), token())
		return e
	})
	if err != nil {
		raw = ""
	}
	return
}
func validLinkSession(ctx context.Context, tx *sql.Tx, hash, accountID string, now time.Time) error {
	var expiry string
	if e := tx.QueryRowContext(ctx, `SELECT expires_at FROM sessions WHERE token_hash=? AND account_id=?`, hash, accountID).Scan(&expiry); e != nil {
		return ErrLinkRequired
	}
	expires, e := time.Parse(time.RFC3339Nano, expiry)
	if e != nil || !expires.After(now) {
		return ErrLinkRequired
	}
	return nil
}

// Package store owns the hosted control database. Brain content never lives here.
package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("hosted record not found")

type Store struct{ db *sql.DB }
type Account struct {
	ID, Email, Status, PlanID string
	PlanVersion               int
}
type Brain struct{ ID, AccountID, State, PathKey string }
type ClientCredential struct {
	ID, BrainID, AccountID, Prefix, Verifier string
	Scopes                                   []string
	Generation                               int
	CreatedAt                                time.Time
	RevokedAt                                *time.Time
}
type CredentialBinding = ClientCredential

func ID() string               { return rand.Text() }
func Hash(value string) string { h := sha256.Sum256([]byte(value)); return hex.EncodeToString(h[:]) }
func Stamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func Open(path string) (*Store, error) {
	u := url.URL{Scheme: "file", Path: path}
	db, err := sql.Open("sqlite", u.String()+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open hosted database: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err = s.Transaction(context.Background(), func(tx *sql.Tx) error { _, e := tx.Exec(schema); return e }); err != nil {
		return nil, errors.Join(fmt.Errorf("migrate hosted database: %w", err), db.Close())
	}
	return s, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) DB() *sql.DB  { return s.db }
func (s *Store) Transaction(ctx context.Context, fn func(*sql.Tx) error) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if e := tx.Rollback(); e != nil && !errors.Is(e, sql.ErrTxDone) {
			err = errors.Join(err, e)
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) CreateAccount(ctx context.Context, email string) (a Account, err error) {
	email = strings.ToLower(strings.TrimSpace(email))
	err = s.Transaction(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `INSERT INTO accounts(id,email_hash,email,created_at,status,plan_id,plan_version) VALUES(?,?,?,?,'active','free',1) ON CONFLICT(email_hash) DO NOTHING`, ID(), Hash(email), email, Stamp(time.Now()))
		if e != nil {
			return e
		}
		return tx.QueryRowContext(ctx, `SELECT id,email,status,plan_id,plan_version FROM accounts WHERE email_hash=?`, Hash(email)).Scan(&a.ID, &a.Email, &a.Status, &a.PlanID, &a.PlanVersion)
	})
	return
}
func (s *Store) InsertBrain(ctx context.Context, accountID, brainID, pathKey, state string, now time.Time) (b Brain, err error) {
	if brainID != pathKey || !validID(brainID) {
		return b, errors.New("invalid brain path key")
	}
	err = s.Transaction(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `INSERT INTO brains(id,account_id,path_key,state,created_at) VALUES(?,?,?,?,?)`, brainID, accountID, pathKey, state, Stamp(now))
		return e
	})
	if err == nil {
		b = Brain{brainID, accountID, state, pathKey}
	}
	return
}
func validID(id string) bool {
	if len(id) < 16 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}
func (s *Store) BrainByID(ctx context.Context, accountID, brainID string) (b Brain, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT id,account_id,state,path_key FROM brains WHERE id=? AND account_id=? AND state!='deleted'`, brainID, accountID).Scan(&b.ID, &b.AccountID, &b.State, &b.PathKey)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return
}
func (s *Store) InsertCredential(ctx context.Context, c ClientCredential) error {
	return s.Transaction(ctx, func(tx *sql.Tx) error { return InsertCredential(ctx, tx, c) })
}
func InsertCredential(ctx context.Context, tx *sql.Tx, c ClientCredential) error {
	if c.Generation < 1 || len(c.Scopes) == 0 {
		return errors.New("invalid credential")
	}
	for _, scope := range c.Scopes {
		if scope != "memory:read" && scope != "memory:write" {
			return errors.New("invalid credential scope")
		}
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO client_credentials(id,brain_id,account_id,prefix,verifier,scopes,generation,created_at) SELECT ?,id,account_id,?,?,?,?,? FROM brains WHERE id=? AND account_id=? AND state='ready'`, c.ID, c.Prefix, c.Verifier, strings.Join(c.Scopes, ","), c.Generation, Stamp(c.CreatedAt), c.BrainID, c.AccountID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrNotFound
	}
	return nil
}
func (s *Store) CredentialByPrefix(ctx context.Context, prefix string) (c CredentialBinding, err error) {
	var scopes string
	var revoked sql.NullString
	err = s.db.QueryRowContext(ctx, `SELECT c.id,c.brain_id,c.account_id,c.prefix,c.verifier,c.scopes,c.generation,c.revoked_at FROM client_credentials c JOIN brains b ON b.id=c.brain_id AND b.account_id=c.account_id JOIN accounts a ON a.id=c.account_id WHERE c.prefix=? AND b.state='ready' AND a.status='active'`, prefix).Scan(&c.ID, &c.BrainID, &c.AccountID, &c.Prefix, &c.Verifier, &scopes, &c.Generation, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, err
	}
	c.Scopes = strings.Split(scopes, ",")
	if revoked.Valid {
		t, e := time.Parse(time.RFC3339Nano, revoked.String)
		if e != nil {
			return c, e
		}
		c.RevokedAt = &t
	}
	return
}

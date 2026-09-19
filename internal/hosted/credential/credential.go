// Package credential issues brain-scoped, revocable MCP credentials.
package credential

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/hosted/store"
)

var ErrInvalidCredential = errors.New("invalid credential")
var ErrRevoked = errors.New("credential revoked")
var ErrLimit = errors.New("active credential limit reached; rotate or revoke an existing connection")

// Covers all ten Scale brains with room for separate client connections.
const MaxActivePerAccount = 16

type Binding struct {
	AccountID, BrainID, CredentialID string
	Generation                       int
	Scopes                           []string
}
type Issuer struct{ Store *store.Store }

func generate(accountID, brainID string, scopes []string, generation int) (string, store.ClientCredential) {
	prefixBytes := make([]byte, 4)
	secretBytes := make([]byte, 32)
	rand.Read(prefixBytes)
	rand.Read(secretBytes)
	prefix := hex.EncodeToString(prefixBytes)
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)
	return "sk_live_" + prefix + "_" + secret, store.ClientCredential{ID: store.ID(), AccountID: accountID, BrainID: brainID, Prefix: prefix, Verifier: store.Hash(secret), Scopes: scopes, Generation: generation, CreatedAt: time.Now()}
}
func (i *Issuer) Issue(ctx context.Context, brainID, accountID string, scopes []string) (string, error) {
	raw, c := generate(accountID, brainID, scopes, 1)
	if err := i.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM client_credentials WHERE account_id=? AND revoked_at IS NULL`, accountID).Scan(&count); err != nil {
			return err
		}
		if count >= MaxActivePerAccount {
			return ErrLimit
		}
		return store.InsertCredential(ctx, tx, c)
	}); err != nil {
		return "", err
	}
	return raw, nil
}
func (i *Issuer) Verify(ctx context.Context, raw string) (Binding, error) {
	var b Binding
	if len(raw) != 60 || !strings.HasPrefix(raw, "sk_live_") || raw[16] != '_' {
		return b, ErrInvalidCredential
	}
	prefix, secret := raw[8:16], raw[17:]
	if _, e := hex.DecodeString(prefix); e != nil {
		return b, ErrInvalidCredential
	}
	v, e := base64.RawURLEncoding.DecodeString(secret)
	if e != nil || len(v) != 32 {
		return b, ErrInvalidCredential
	}
	c, err := i.Store.CredentialByPrefix(ctx, prefix)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return b, ErrInvalidCredential
		}
		return b, err
	}
	if subtle.ConstantTimeCompare([]byte(store.Hash(secret)), []byte(c.Verifier)) != 1 {
		return b, ErrInvalidCredential
	}
	if c.RevokedAt != nil {
		return b, ErrRevoked
	}
	return Binding{c.AccountID, c.BrainID, c.ID, c.Generation, c.Scopes}, nil
}

// Rotate requires account ownership even for internal callers.
func (i *Issuer) Rotate(ctx context.Context, accountID, brainID string) (raw string, err error) {
	err = i.Store.Transaction(ctx, func(tx *sql.Tx) error {
		var scopes string
		var generation int
		e := tx.QueryRowContext(ctx, `SELECT scopes,generation FROM client_credentials WHERE brain_id=? AND account_id=? AND revoked_at IS NULL ORDER BY generation DESC LIMIT 1`, brainID, accountID).Scan(&scopes, &generation)
		if errors.Is(e, sql.ErrNoRows) {
			return store.ErrNotFound
		}
		if e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `UPDATE client_credentials SET revoked_at=? WHERE brain_id=? AND account_id=? AND revoked_at IS NULL`, store.Stamp(time.Now()), brainID, accountID)
		if e != nil {
			return e
		}
		var c store.ClientCredential
		raw, c = generate(accountID, brainID, strings.Split(scopes, ","), generation+1)
		return store.InsertCredential(ctx, tx, c)
	})
	if err != nil {
		raw = ""
	}
	return
}
func (i *Issuer) Revoke(ctx context.Context, accountID, brainID string) error {
	if _, err := i.Store.BrainByID(ctx, accountID, brainID); err != nil {
		return err
	}
	return i.Store.Transaction(ctx, func(tx *sql.Tx) error {
		_, e := tx.ExecContext(ctx, `UPDATE client_credentials SET revoked_at=? WHERE brain_id=? AND account_id=? AND revoked_at IS NULL`, store.Stamp(time.Now()), brainID, accountID)
		return e
	})
}

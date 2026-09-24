// Package oauth adapts AMSL OAuth to hosted account and project authorization.
package oauth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/ajent-social/go/mcpoauth"
	"github.com/sirerun/serenity/internal/hosted/store"
)

// SQLStore keeps OAuth records in the same backed-up control database as accounts.
// One-time consumption uses DELETE RETURNING, so independent processes cannot
// both redeem a consent handle or authorization code.
type SQLStore struct{ DB *store.Store }

func (s *SQLStore) create(ctx context.Context, table, id string, value any, expires time.Time) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.DB.Transaction(ctx, func(tx *sql.Tx) error {

		// Table names are internal constants, never request input. Expiry bounds
		// unauthenticated registration/consent storage; semantic checks remain in AMSL.
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE expires_at<?", store.Stamp(time.Now())); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
			return err
		}
		if table == "oauth_clients" && count >= 10000 {
			// Unary + removes TEXT affinity from c.id so SQLite can use the
			// client_id expression indexes in these correlated subqueries.
			// Only unreferenced registrations may be evicted. Pending consent,
			// codes and live grants protect their registered callbacks.
			result, err := tx.ExecContext(ctx, `DELETE FROM oauth_clients WHERE id IN (SELECT c.id FROM oauth_clients c WHERE NOT EXISTS(SELECT 1 FROM oauth_grants g WHERE json_extract(g.record,'$.client_id')=+c.id AND g.expires_at>?) AND NOT EXISTS(SELECT 1 FROM oauth_consents p WHERE json_extract(p.record,'$.client_id')=+c.id AND p.expires_at>?) AND NOT EXISTS(SELECT 1 FROM oauth_codes p WHERE json_extract(p.record,'$.client_id')=+c.id AND p.expires_at>?) ORDER BY c.expires_at LIMIT 100)`, store.Stamp(time.Now()), store.Stamp(time.Now()), store.Stamp(time.Now()))
			if err != nil {
				return err
			}
			removed, err := result.RowsAffected()
			if err != nil {
				return err
			}
			count -= int(removed)
		}
		if count >= 10000 {
			return errors.New("oauth storage capacity reached")
		}
		var clientID string
		switch rec := value.(type) {
		case mcpoauth.ConsentRecord:
			clientID = rec.ClientID
		case mcpoauth.CodeRecord:
			clientID = rec.ClientID
		}
		if clientID != "" {
			if _, err := tx.ExecContext(ctx, "UPDATE oauth_clients SET expires_at=MAX(expires_at,?) WHERE id=?", store.Stamp(expires.Add(10*time.Minute)), clientID); err != nil {
				return err
			}
		}
		result, err := tx.ExecContext(ctx, "INSERT INTO "+table+"(id,record,expires_at) VALUES(?,?,?) ON CONFLICT(id) DO NOTHING", id, string(body), store.Stamp(expires))
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return mcpoauth.ErrExists
		}
		return nil
	})
}
func (s *SQLStore) read(ctx context.Context, table, id string, out any, consume bool) error {
	var body string
	query := "SELECT record FROM " + table + " WHERE id=?"
	if consume {
		query = "DELETE FROM " + table + " WHERE id=? RETURNING record"
	}
	err := s.DB.DB().QueryRowContext(ctx, query, id).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return mcpoauth.ErrNotFound
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(body), out)
}
func (s *SQLStore) CreateClient(ctx context.Context, c mcpoauth.Client) error {
	return s.create(ctx, "oauth_clients", c.ID, c, c.CreatedAt.Add(time.Hour))
}
func (s *SQLStore) Client(ctx context.Context, id string) (v mcpoauth.Client, err error) {
	err = s.read(ctx, "oauth_clients", id, &v, false)
	return
}
func (s *SQLStore) CreateConsent(ctx context.Context, c mcpoauth.ConsentRecord) error {
	return s.create(ctx, "oauth_consents", c.ID, c, c.ExpiresAt)
}
func (s *SQLStore) Consent(ctx context.Context, id string) (v mcpoauth.ConsentRecord, err error) {
	err = s.read(ctx, "oauth_consents", id, &v, false)
	return
}
func (s *SQLStore) ConsumeConsent(ctx context.Context, id string) (v mcpoauth.ConsentRecord, err error) {
	err = s.read(ctx, "oauth_consents", id, &v, true)
	return
}
func (s *SQLStore) CreateCode(ctx context.Context, c mcpoauth.CodeRecord) error {
	return s.create(ctx, "oauth_codes", c.ID, c, c.ExpiresAt)
}
func (s *SQLStore) ConsumeCode(ctx context.Context, id string) (v mcpoauth.CodeRecord, err error) {
	err = s.read(ctx, "oauth_codes", id, &v, true)
	return
}

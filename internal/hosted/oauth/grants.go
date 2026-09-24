package oauth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/ajent-social/go/mcpoauth"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func (s *SQLStore) Grant(ctx context.Context, id string) (v mcpoauth.GrantRecord, err error) {
	err = s.read(ctx, "oauth_grants", id, &v, false)
	return
}
func (s *SQLStore) Token(ctx context.Context, id string) (v mcpoauth.TokenRecord, err error) {
	err = s.read(ctx, "oauth_tokens", id, &v, false)
	return
}
func (s *SQLStore) Refresh(ctx context.Context, id string) (v mcpoauth.RefreshRecord, err error) {
	err = s.read(ctx, "oauth_refresh", id, &v, false)
	return
}
func readTx(ctx context.Context, tx *sql.Tx, table, id string, out any) error {
	var raw string
	err := tx.QueryRowContext(ctx, "SELECT record FROM "+table+" WHERE id=?", id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return mcpoauth.ErrNotFound
	}
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), out)
}
func insertTx(ctx context.Context, tx *sql.Tx, table, id, grant string, v any, expires time.Time) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var result sql.Result
	if grant == "" {
		result, err = tx.ExecContext(ctx, "INSERT INTO "+table+"(id,record,expires_at) VALUES(?,?,?) ON CONFLICT(id) DO NOTHING", id, string(raw), store.Stamp(expires))
	} else {
		result, err = tx.ExecContext(ctx, "INSERT INTO "+table+"(id,grant_id,record,expires_at) VALUES(?,?,?,?) ON CONFLICT(id) DO NOTHING", id, grant, string(raw), store.Stamp(expires))
	}
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
}
func updateTx(ctx context.Context, tx *sql.Tx, table, id string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, "UPDATE "+table+" SET record=? WHERE id=?", string(raw), id)
	return err
}
func livePolicy(ctx context.Context, tx *sql.Tx, g mcpoauth.GrantRecord) error {
	brain, generation, valid := strings.Cut(g.Binding, ".")
	if !valid {
		return mcpoauth.ErrDenied
	}
	var status, state string
	var epoch int
	err := tx.QueryRowContext(ctx, `SELECT a.status,b.state,COALESCE(e.generation,0) FROM brains b JOIN accounts a ON a.id=b.account_id LEFT JOIN oauth_epochs e ON e.brain_id=b.id WHERE a.id=? AND b.id=?`, g.Subject, brain).Scan(&status, &state, &epoch)
	if err != nil || status != "active" || state != "ready" || generation != strconv.Itoa(epoch) {
		return mcpoauth.ErrDenied
	}
	return nil
}

// lockWriter acquires SQLite's writer lock before reading lifecycle state.
// This also works across independent Store instances/processes.
func lockWriter(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, "UPDATE oauth_grants SET expires_at=expires_at WHERE id='' ")
	return err
}
func (s *SQLStore) CreateGrant(ctx context.Context, issue mcpoauth.GrantIssue) error {
	return s.DB.Transaction(ctx, func(tx *sql.Tx) error {
		if err := lockWriter(ctx, tx); err != nil {
			return err
		}
		if err := livePolicy(ctx, tx, issue.Grant); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM oauth_grants WHERE expires_at<?", store.Stamp(time.Now())); err != nil {
			return err
		}
		// Re-authorization replaces this client's prior connection to the
		// same project, including its old scope grant and refresh family.
		if _, err := tx.ExecContext(ctx, "DELETE FROM oauth_grants WHERE json_extract(record,'$.subject')=? AND json_extract(record,'$.binding')=? AND json_extract(record,'$.client_id')=?", issue.Grant.Subject, issue.Grant.Binding, issue.Grant.ClientID); err != nil {
			return err
		}
		var active int
		if err := tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM oauth_grants WHERE json_extract(record,'$.subject')=? AND json_extract(record,'$.revoked_at') IS NULL AND expires_at>?)+(SELECT count(*) FROM client_credentials WHERE account_id=? AND revoked_at IS NULL)`, issue.Grant.Subject, store.Stamp(time.Now()), issue.Grant.Subject).Scan(&active); err != nil {
			return err
		}
		if active >= 16 {
			return mcpoauth.ErrDenied
		}
		if issue.Token.GrantID != issue.Grant.ID || (issue.Refresh != nil && issue.Refresh.GrantID != issue.Grant.ID) {
			return mcpoauth.ErrInvalid
		}
		if _, err := tx.ExecContext(ctx, "UPDATE oauth_clients SET expires_at=MAX(expires_at,?) WHERE id=?", store.Stamp(issue.Grant.ExpiresAt.Add(7*24*time.Hour)), issue.Grant.ClientID); err != nil {
			return err
		}
		if err := insertTx(ctx, tx, "oauth_grants", issue.Grant.ID, "", issue.Grant, issue.Grant.ExpiresAt); err != nil {
			return err
		}
		if err := insertTx(ctx, tx, "oauth_tokens", issue.Token.ID, issue.Grant.ID, issue.Token, issue.Token.ExpiresAt); err != nil {
			return err
		}
		if issue.Refresh != nil {
			return insertTx(ctx, tx, "oauth_refresh", issue.Refresh.ID, issue.Grant.ID, issue.Refresh, issue.Refresh.ExpiresAt)
		}
		return nil
	})
}
func (s *SQLStore) RotateRefresh(ctx context.Context, r mcpoauth.RefreshRotation) error {
	reused := false
	err := s.DB.Transaction(ctx, func(tx *sql.Tx) error {
		if err := lockWriter(ctx, tx); err != nil {
			return err
		}
		var old mcpoauth.RefreshRecord
		if err := readTx(ctx, tx, "oauth_refresh", r.RefreshID, &old); err != nil {
			return err
		}
		var grant mcpoauth.GrantRecord
		if err := readTx(ctx, tx, "oauth_grants", old.GrantID, &grant); err != nil {
			return err
		}
		if !old.UsedAt.IsZero() {
			reused = true
			if grant.RevokedAt.IsZero() {
				grant.RevokedAt = r.At
			}
			return updateTx(ctx, tx, "oauth_grants", grant.ID, grant)
		}
		if !grant.RevokedAt.IsZero() || !r.At.Before(grant.ExpiresAt) || !r.At.Before(old.ExpiresAt) {
			return mcpoauth.ErrDenied
		}
		if r.Token.GrantID != grant.ID || r.Refresh.GrantID != grant.ID || r.Refresh.Generation != old.Generation+1 {
			return mcpoauth.ErrInvalid
		}
		if err := livePolicy(ctx, tx, grant); err != nil {
			return err
		}
		// A 30-day grant supports normal hourly refreshes (720). Bound abusive
		// rotation without discarding used hashes needed for replay detection.
		if r.Refresh.Generation > 1024 {
			return mcpoauth.ErrDenied
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM oauth_tokens WHERE grant_id=? AND expires_at<?", grant.ID, store.Stamp(r.At)); err != nil {
			return err
		}
		if err := insertTx(ctx, tx, "oauth_tokens", r.Token.ID, grant.ID, r.Token, r.Token.ExpiresAt); err != nil {
			return err
		}
		if err := insertTx(ctx, tx, "oauth_refresh", r.Refresh.ID, grant.ID, r.Refresh, r.Refresh.ExpiresAt); err != nil {
			return err
		}
		old.UsedAt = r.At
		old.ReplacedBy = r.Refresh.ID
		return updateTx(ctx, tx, "oauth_refresh", old.ID, old)
	})
	if err != nil {
		return err
	}
	if reused {
		return mcpoauth.ErrReused
	}
	return nil
}
func (s *SQLStore) RevokeGrant(ctx context.Context, id string, at time.Time) error {
	return s.DB.Transaction(ctx, func(tx *sql.Tx) error {
		if err := lockWriter(ctx, tx); err != nil {
			return err
		}
		var grant mcpoauth.GrantRecord
		if err := readTx(ctx, tx, "oauth_grants", id, &grant); err != nil {
			return err
		}
		if !grant.RevokedAt.IsZero() {
			return nil
		}
		grant.RevokedAt = at
		return updateTx(ctx, tx, "oauth_grants", id, grant)
	})
}

package store

import (
	"context"
	"database/sql"
)

// RevokeOAuth advances the brain's authority epoch in the same transaction as
// legacy credential revocation. Tokens and even concurrently redeemed old
// authorization codes then fail verification, without cross-store races.
func RevokeOAuth(ctx context.Context, tx *sql.Tx, account, brain string) error {
	var owned int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM brains WHERE account_id=? AND id=?", account, brain).Scan(&owned); err != nil {
		return err
	}
	if owned != 1 {
		return ErrNotFound
	}
	_, err := tx.ExecContext(ctx, "INSERT INTO oauth_epochs(brain_id,generation) VALUES(?,1) ON CONFLICT(brain_id) DO UPDATE SET generation=generation+1", brain)
	if err != nil {
		return err
	}
	for _, table := range []string{"oauth_grants", "oauth_codes"} {
		if _, err = tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE json_extract(record,'$.subject')=? AND substr(json_extract(record,'$.binding'),1,?)=?", account, len(brain)+1, brain+"."); err != nil {
			return err
		}
	}
	return nil
}

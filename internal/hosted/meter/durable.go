package meter

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// DurableCommitted is a read-only acknowledgement fence. A durable source alone
// is not sufficient evidence that its original quota admission was settled.
func (m *Meter) DurableCommitted(ctx context.Context, account, metric, key string) (bool, error) {
	if !strings.HasPrefix(key, "governed:") {
		return false, errors.New("invalid durable operation key")
	}
	var count, committed int
	err := m.Store.DB().QueryRowContext(ctx, "SELECT count(*),COALESCE(sum(CASE WHEN status='committed' THEN 1 ELSE 0 END),0) FROM reservations WHERE account_id=? AND metric=? AND operation_key=?", account, metric, key).Scan(&count, &committed)
	return count == 1 && committed == 1, err
}

// CommitDurableOperation settles the original admission, including a historical
// expired/released row. It does not create a reservation or consult today's
// quota: the already durable operation must be charged to its original window.
func (m *Meter) CommitDurableOperation(ctx context.Context, account, metric, key string) error {
	if !strings.HasPrefix(key, "governed:") {
		return errors.New("invalid durable operation key")
	}
	return m.Store.Transaction(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT id,window_key,amount,status FROM reservations WHERE account_id=? AND metric=? AND operation_key=?", account, metric, key)
		if err != nil {
			return err
		}
		var id, window, status string
		var amount int64
		count := 0
		for rows.Next() {
			count++
			if err = rows.Scan(&id, &window, &amount, &status); err != nil {
				_ = rows.Close()
				return err
			}
		}
		err = rows.Err()
		closeErr := rows.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if count != 1 || amount != 1 {
			return fmt.Errorf("durable reservation missing or ambiguous")
		}
		if status == "committed" {
			return nil
		}
		if status != "open" && status != "released" {
			return fmt.Errorf("invalid durable reservation state")
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO usage_windows(account_id,window_key,metric,committed) VALUES(?,?,?,?) ON CONFLICT(account_id,window_key,metric) DO UPDATE SET committed=committed+excluded.committed", account, window, metric, amount); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE reservations SET status='committed' WHERE id=?", id)
		return err
	})
}

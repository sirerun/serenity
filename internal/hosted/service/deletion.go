package service

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

// DeleteAccount freezes access before resolving provider closure. Uncertain
// closure retains the account and brain files for a later deletion retry.
func (s *Service) DeleteAccount(ctx context.Context, accountID string) error {
	var status, customer string
	var subscriptions bool
	err := s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `UPDATE accounts SET status='deleting' WHERE id=? AND status='active'`, accountID); err != nil {
			return err
		}
		return tx.QueryRowContext(ctx, `SELECT status,COALESCE(stripe_customer_id,''),EXISTS(SELECT 1 FROM subscriptions WHERE account_id=accounts.id) FROM accounts WHERE id=?`, accountID).Scan(&status, &customer, &subscriptions)
	})
	if err != nil {
		return err
	}
	if status == "deleted" {
		return nil
	}
	if status != "deleting" {
		return contracts.ErrBillingAccountFrozen
	}
	if s.billingCloser != nil {
		result, err := s.billingCloser.CloseBillingAccount(ctx, accountID)
		if err != nil {
			return err
		}
		if result.Status != contracts.CloseStatusClosed {
			return errors.New("hosted: billing closure is pending")
		}
	} else if s.cfg.BillingEnabled || customer != "" || subscriptions {
		return errors.New("hosted: billing closure is unavailable")
	}
	return s.Gateway.DeleteAccount(ctx, accountID, filepath.Join(s.cfg.DataDir, "brains"))
}

func (s *Service) recoverDeletions(ctx context.Context) error {
	rows, err := s.Store.DB().QueryContext(ctx, `SELECT id FROM accounts WHERE status='deleting'`)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return errors.Join(err, rows.Close())
		}
		ids = append(ids, id)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.DeleteAccount(ctx, id); err != nil {
			return err
		}
	}
	// After provider closure has been certified for every deleting account,
	// retain the existing cleanup of already-deleted brain directories.
	return s.Gateway.RecoverDeletions(ctx, filepath.Join(s.cfg.DataDir, "brains"))
}

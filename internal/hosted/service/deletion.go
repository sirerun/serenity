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
	return s.Gateway.DeleteAccountWithPreflight(ctx, accountID, filepath.Join(s.cfg.DataDir, "brains"), s.accountDeletionPreflight)
}

func (s *Service) accountDeletionPreflight(ctx context.Context, accountID string) (bool, error) {
	var status, customer string
	var subscriptions bool
	var alreadyDeleted bool
	err := s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `SELECT status FROM accounts WHERE id=?`, accountID).Scan(&status); err != nil {
			return err
		}
		if status == "deleted" {
			alreadyDeleted = true
		} else if status != "active" && status != "deleting" {
			return contracts.ErrBillingAccountFrozen
		} else {
			if _, err := tx.ExecContext(ctx, `UPDATE accounts SET status='deleting' WHERE id=? AND status='active'`, accountID); err != nil {
				return err
			}
		}
		return tx.QueryRowContext(ctx, `SELECT status,COALESCE(stripe_customer_id,''),EXISTS(SELECT 1 FROM subscriptions WHERE account_id=accounts.id) FROM accounts WHERE id=?`, accountID).Scan(&status, &customer, &subscriptions)
	})
	if err != nil {
		return false, err
	}
	if status != "deleting" && status != "deleted" {
		return false, contracts.ErrBillingAccountFrozen
	}
	if s.billingCloser != nil {
		result, err := s.billingCloser.CloseBillingAccount(ctx, accountID)
		if err != nil {
			return false, err
		}
		if result.Status != contracts.CloseStatusClosed {
			return false, errors.New("hosted: billing closure is pending")
		}
	} else if s.cfg.BillingEnabled || customer != "" || subscriptions {
		return false, errors.New("hosted: billing closure is unavailable")
	}
	if alreadyDeleted {
		return false, nil
	}
	return true, nil
}

func (s *Service) recoverDeletions(ctx context.Context, entries []contracts.DeletionEntry) error {
	return s.Gateway.ReplayDeletions(ctx, filepath.Join(s.cfg.DataDir, "brains"), entries, s.accountDeletionPreflight)
}

package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/store"
)

func verifyPendingDeletionIntents(ctx context.Context, db *store.Store, entries []contracts.DeletionEntry) (retErr error) {
	if db == nil {
		return errors.New("hosted: control store is required to verify pending deletion intents")
	}
	requested := make(map[string]struct{})
	for _, entry := range entries {
		if entry.SubjectType == contracts.DeletionSubjectAccount && entry.Outcome == contracts.DeletionIntentRequested {
			requested[entry.SubjectID] = struct{}{}
		}
	}
	rows, err := db.DB().QueryContext(ctx, `SELECT id FROM accounts WHERE status='deleting'`)
	if err != nil {
		return fmt.Errorf("hosted: inspect pending account deletions: %w", err)
	}
	defer func() { retErr = errors.Join(retErr, rows.Close()) }()
	for rows.Next() {
		var accountID string
		if err := rows.Scan(&accountID); err != nil {
			return fmt.Errorf("hosted: read pending account deletion: %w", err)
		}
		if _, ok := requested[accountID]; !ok {
			return fmt.Errorf("%w: pending account deletion has no verified journal intent", contracts.ErrDeletionJournalIncomplete)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("hosted: finish pending deletion scan: %w", err)
	}
	return nil
}

// DeleteAccount freezes access before resolving provider closure. Uncertain
// closure retains the account and brain files for a later deletion retry.
func (s *Service) DeleteAccount(ctx context.Context, accountID string) error {
	return s.Gateway.DeleteAccountWithPreflight(ctx, accountID, filepath.Join(s.cfg.DataDir, "brains"), s.accountDeletionPreflight)
}

func (s *Service) accountDeletionPreflight(ctx context.Context, accountID string) (bool, error) {
	return s.deletionPreflight(ctx, accountID, false)
}

// deletionPreflight permits restored frozen state only for admitted startup replay.
func (s *Service) deletionPreflight(ctx context.Context, accountID string, admittedReplay bool) (bool, error) {
	var status, customer string
	var subscriptions bool
	err := s.Store.Transaction(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `SELECT status FROM accounts WHERE id=?`, accountID).Scan(&status); err != nil {
			return err
		}
		if status != "deleted" && status != "active" && status != "deleting" && (!admittedReplay || status != "restore_pending") {
			return contracts.ErrBillingAccountFrozen
		} else {
			if _, err := tx.ExecContext(ctx, `UPDATE accounts SET status='deleting' WHERE id=? AND (status IN ('active','deleted') OR (? AND status='restore_pending'))`, accountID, admittedReplay); err != nil {
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
	return true, nil
}

func (s *Service) recoverDeletions(ctx context.Context, entries []contracts.DeletionEntry) error {
	return s.Gateway.ReplayDeletions(ctx, filepath.Join(s.cfg.DataDir, "brains"), entries, func(replayCtx context.Context, accountID string) (bool, error) {
		return s.deletionPreflight(replayCtx, accountID, true)
	})
}

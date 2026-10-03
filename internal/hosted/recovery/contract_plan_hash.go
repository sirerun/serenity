package recovery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

const maxCanonicalContractPlanBytes = 1 << 20

const contractPlanValidationHash = "0000000000000000000000000000000000000000000000000000000000000000"

type recoveryContractPlanHashPayload struct {
	DomainVersion    int                         `json:"domain_version"`
	SourceSnapshot   string                      `json:"source_snapshot"`
	JournalWatermark contracts.DeletionWatermark `json:"journal_watermark"`
	Generation       int64                       `json:"generation"`
	ProviderTruthAt  string                      `json:"provider_truth_at"`
	Accounts         []string                    `json:"accounts"`
}

// CanonicalContractRecoveryPlanHash computes the version-1 contract-plan
// digest. PlanHash is ignored and excluded from the hashed payload.
func CanonicalContractRecoveryPlanHash(plan contracts.RecoveryPlan) (RecoveryContractPlanHash, error) {
	if len(plan.Accounts) > maxVerifiedEligibleAccounts {
		return "", fmt.Errorf("%w: contract eligible account scope exceeds %d", ErrPlanInvalid, maxVerifiedEligibleAccounts)
	}
	if plan.ProviderTruthAt.Location() != time.UTC {
		return "", fmt.Errorf("%w: contract provider truth time must use UTC", ErrPlanInvalid)
	}
	// Reuse the stricter existing artifact account validator before allocating
	// a copy, then the contract's unchanged semantic validator below.
	projection := Plan{
		FormatVersion:    planFormatVersion,
		SnapshotSHA256:   plan.SourceSnapshot,
		JournalWatermark: plan.JournalWatermark,
		FenceGeneration:  plan.Generation,
		ProviderObserved: plan.ProviderTruthAt.Format(time.RFC3339Nano),
		Accounts:         plan.Accounts,
	}
	if err := validatePlanPayload(projection); err != nil {
		return "", errors.Join(ErrPlanInvalid, err)
	}
	accounts := append([]string(nil), plan.Accounts...)
	validationCopy := plan
	validationCopy.Accounts = accounts
	// This placeholder exists only in a local validation copy. It is never
	// returned, serialized, or persisted.
	validationCopy.PlanHash = contractPlanValidationHash
	if err := validationCopy.Validate(); err != nil {
		return "", errors.Join(ErrPlanInvalid, err)
	}
	payload := recoveryContractPlanHashPayload{
		DomainVersion:    1,
		SourceSnapshot:   plan.SourceSnapshot,
		JournalWatermark: plan.JournalWatermark,
		Generation:       plan.Generation,
		ProviderTruthAt:  plan.ProviderTruthAt.Format(time.RFC3339Nano),
		Accounts:         accounts,
	}
	canonical, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal canonical recovery contract plan: %w", err)
	}
	if len(canonical) > maxCanonicalContractPlanBytes {
		return "", fmt.Errorf("%w: canonical contract plan exceeds size limit", ErrPlanInvalid)
	}
	digest := sha256.Sum256(canonical)
	return RecoveryContractPlanHash(hex.EncodeToString(digest[:])), nil
}

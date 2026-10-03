package recovery

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

// ValidateRecoveryEnvelopeV1 validates the canonical-data shape and all
// source-independent relationships. It does not authenticate any field.
func ValidateRecoveryEnvelopeV1(ctx context.Context, in RecoveryEnvelopeV1) error {
	if err := ensureContext(ctx); err != nil {
		return err
	}
	if err := validateEnvelopeFields(ctx, in); err != nil {
		return err
	}
	_, err := canonicalJSON(toEnvelopeWire(in), maxEnvelopeBytes)
	if err != nil {
		if errors.Is(err, ErrRecoveryEnvelopeTooLarge) {
			return err
		}
		return fmt.Errorf("%w: encode envelope: %v", ErrRecoveryEnvelopeInvalid, err)
	}
	if err := ensureContext(ctx); err != nil {
		return err
	}
	return nil
}

func validateEnvelopeFields(ctx context.Context, in RecoveryEnvelopeV1) error {
	if in.FormatVersion != envelopeFormatVersion {
		return fmt.Errorf("%w: unsupported format version", ErrRecoveryEnvelopeInvalid)
	}
	if err := ensureContext(ctx); err != nil {
		return err
	}
	if !validASCIIComponent(in.PlanRef, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._:/-") ||
		!validASCIIComponent(in.SnapshotPinID, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._:/-") ||
		in.ReservationVersion < 1 {
		return fmt.Errorf("%w: invalid plan reservation identity", ErrRecoveryEnvelopeInvalid)
	}
	if !validHash(in.ManifestSHA256) || !validText(in.SourceBuildToken, maxEnvelopeSourceTokenBytes) || in.SourceSchemaVersion < 1 || in.SourceSchemaVersion > math.MaxInt32 {
		return fmt.Errorf("%w: invalid snapshot source identity", ErrRecoveryEnvelopeInvalid)
	}
	if err := validAccountRows(in.SnapshotAccountInventory); err != nil {
		return err
	}
	if in.ControlDBLength <= 0 || in.ControlDBLength > maxEnvelopeArtifactBytes || in.VerifiedArtifactCount < 1 || in.VerifiedArtifactCount > maxEnvelopeArtifactCount || in.DeclaredArtifactBytes < in.ControlDBLength || in.DeclaredArtifactBytes > maxEnvelopeArtifactBytes {
		return fmt.Errorf("%w: invalid artifact summary bounds", ErrRecoveryEnvelopeInvalid)
	}
	if in.SnapshotBrainInventory.Count < 0 || in.SnapshotBrainInventory.Count > maxSnapshotBrains || !validHash(in.SnapshotBrainInventory.CanonicalDigest) {
		return fmt.Errorf("%w: invalid brain inventory summary", ErrRecoveryEnvelopeInvalid)
	}
	if err := validateSortedAccountIDs(in.ActivationAllowlist, maxEnvelopeEligibleAccounts); err != nil {
		return fmt.Errorf("%w: invalid activation allowlist: %v", ErrRecoveryEnvelopeInvalid, err)
	}
	if !validASCIIComponent(in.OperationID, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._:/-") {
		return fmt.Errorf("%w: invalid operation ID", ErrRecoveryEnvelopeInvalid)
	}
	if err := validateEvidenceRef(in.PlanApprovalRef); err != nil {
		return err
	}
	if err := validateEvidenceRef(in.PrefixEvidenceRef); err != nil {
		return err
	}
	if !validHash(in.PlanApprovalDigest) || !validASCIIComponent(in.PlanApprovalNonce, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._:/-") {
		return fmt.Errorf("%w: invalid plan approval binding data", ErrRecoveryEnvelopeInvalid)
	}
	if in.PlanApprovalExpiresAt.IsZero() || in.PlanApprovalExpiresAt.Location() != time.UTC || len(in.PlanApprovalExpiresAt.Format(time.RFC3339Nano)) > 35 {
		return fmt.Errorf("%w: plan approval expiry must be canonical UTC", ErrRecoveryEnvelopeInvalid)
	}
	if _, err := parseCanonicalUTC(in.PlanApprovalExpiresAt.Format(time.RFC3339Nano)); err != nil {
		return err
	}
	if !validASCIIComponent(in.OldWriter.Provider, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._:/-") ||
		!validASCIIComponent(in.OldWriter.ScopeRef, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._:/-") ||
		!validASCIIComponent(in.OldWriter.WriterRef, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._:/-") ||
		!validASCIIComponent(in.OldWriter.BootRef, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._:/-") ||
		!validASCIIComponent(in.OldWriter.JournalStoreID, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._:/-") ||
		in.OldWriter.Generation < 1 || in.OldWriter.Generation > 10_000 ||
		!validASCIIComponent(in.JournalStoreID, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._:/-") ||
		in.JournalStoreID != in.OldWriter.JournalStoreID {
		return fmt.Errorf("%w: invalid or mismatched old writer/store identity", ErrRecoveryEnvelopeInvalid)
	}
	if !validJournalPositionV1(in.SnapshotCut) || in.SnapshotCut.ActiveGeneration > in.OldWriter.Generation {
		return fmt.Errorf("%w: invalid or future snapshot cut", ErrRecoveryEnvelopeInvalid)
	}
	if !isZeroOrValidWatermark(in.ManifestJournalWatermark) || !watermarkNoLaterThanPosition(in.ManifestJournalWatermark, in.SnapshotCut) {
		return fmt.Errorf("%w: invalid or future manifest journal watermark", ErrRecoveryEnvelopeInvalid)
	}
	if !isZeroOrValidWatermark(in.LastSealed) || !isZeroWatermark(in.LastSealed) && in.LastSealed.Generation > in.OldWriter.Generation {
		return fmt.Errorf("%w: invalid historical seal", ErrRecoveryEnvelopeInvalid)
	}
	if err := validateInventoryDigest(ctx, in); err != nil {
		return err
	}
	if err := ensureContext(ctx); err != nil {
		return err
	}
	switch in.Kind {
	case RecoveryEnvelopeEligible:
		if in.Eligible == nil || in.Frozen != nil {
			return fmt.Errorf("%w: eligible union requires only eligible arm", ErrRecoveryEnvelopeInvalid)
		}
		if err := validateEligibleArm(in); err != nil {
			return err
		}
	case RecoveryEnvelopeFrozenOnly:
		if in.Frozen == nil || in.Eligible != nil {
			return fmt.Errorf("%w: frozen union requires only frozen arm", ErrRecoveryEnvelopeInvalid)
		}
		if err := validateFrozenArm(in); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%w: unknown envelope kind", ErrRecoveryEnvelopeInvalid)
	}
	if evidenceRefByteLength(in.PlanApprovalRef)+evidenceRefByteLength(in.PrefixEvidenceRef) > maxEnvelopeEvidenceBytes {
		return fmt.Errorf("%w: evidence reference bytes exceed limit", ErrRecoveryEnvelopeTooLarge)
	}
	return nil
}

func validateInventoryDigest(ctx context.Context, in RecoveryEnvelopeV1) error {
	if !validHash(in.SnapshotInventorySHA256) {
		return fmt.Errorf("%w: invalid inventory digest", ErrRecoveryEnvelopeInvalid)
	}
	summary := SnapshotInventorySummaryV1{
		ManifestSHA256:        in.ManifestSHA256,
		SourceBuildToken:      in.SourceBuildToken,
		SourceSchemaVersion:   in.SourceSchemaVersion,
		Accounts:              in.SnapshotAccountInventory,
		ControlDBLength:       in.ControlDBLength,
		BrainCount:            in.SnapshotBrainInventory.Count,
		BrainInventorySHA256:  in.SnapshotBrainInventory.CanonicalDigest,
		VerifiedArtifactCount: in.VerifiedArtifactCount,
		DeclaredArtifactBytes: in.DeclaredArtifactBytes,
	}
	digest, err := SnapshotInventorySummaryDigestV1(ctx, summary)
	if err != nil {
		return err
	}
	if digest != in.SnapshotInventorySHA256 {
		return fmt.Errorf("%w: snapshot inventory digest mismatch", ErrRecoveryEnvelopeInvalid)
	}
	return nil
}

func validateEligibleArm(in RecoveryEnvelopeV1) error {
	arm := in.Eligible
	if arm == nil || !validHash(arm.LegacyPlanArtifactHash) || !validHash(arm.RecoveryContractPlanHash) {
		return fmt.Errorf("%w: eligible arm hashes invalid", ErrRecoveryEnvelopeInvalid)
	}
	if !sortedUniqueStrings(arm.EligibleAccountIDs, maxEnvelopeEligibleAccounts, validSnapshotAccountID) ||
		!slices.Equal(arm.EligibleAccountIDs, arm.ContractPlan.Accounts) || len(arm.EligibleAccountIDs) == 0 {
		return fmt.Errorf("%w: eligible IDs must be nonempty, sorted and equal to plan accounts", ErrRecoveryEnvelopeInvalid)
	}
	for _, id := range arm.EligibleAccountIDs {
		if !containsSorted(in.ActivationAllowlist, id) {
			return fmt.Errorf("%w: eligible account is outside activation allowlist", ErrRecoveryEnvelopeInvalid)
		}
		idx, ok := slices.BinarySearchFunc(in.SnapshotAccountInventory, id, func(row SnapshotAccountV1, target string) int { return strings.Compare(row.ID, target) })
		if !ok || idx >= len(in.SnapshotAccountInventory) {
			return fmt.Errorf("%w: eligible account is absent from snapshot inventory", ErrRecoveryEnvelopeInvalid)
		}
		status := in.SnapshotAccountInventory[idx].Status
		if status != "active" && status != "restore_pending" {
			return fmt.Errorf("%w: snapshot account status is not eligible", ErrRecoveryEnvelopeInvalid)
		}
	}
	plan := arm.ContractPlan
	if !validHash(plan.SourceSnapshot) || plan.SourceSnapshot != in.ManifestSHA256 ||
		plan.JournalWatermark != in.ManifestJournalWatermark || plan.Generation != in.OldWriter.Generation ||
		plan.ProviderTruthAt.IsZero() || plan.ProviderTruthAt.Location() != time.UTC || len(plan.Accounts) > maxEnvelopeEligibleAccounts {
		return fmt.Errorf("%w: contract plan projection disagrees with envelope", ErrRecoveryEnvelopeInvalid)
	}
	providerAt := plan.ProviderTruthAt.Format(time.RFC3339Nano)
	if _, err := parseCanonicalUTC(providerAt); err != nil {
		return err
	}
	canonicalPlan, err := canonicalJSON(contractPlanV1Wire{
		SourceSnapshot:   plan.SourceSnapshot,
		JournalWatermark: plan.JournalWatermark.wire(),
		Generation:       plan.Generation,
		ProviderTruthAt:  providerAt,
		Accounts:         cloneSlice(plan.Accounts),
	}, maxEnvelopePlanBytes)
	if err != nil {
		return err
	}
	_ = canonicalPlan
	contractPlan := contracts.RecoveryPlan{
		PlanHash:         arm.RecoveryContractPlanHash,
		SourceSnapshot:   plan.SourceSnapshot,
		JournalWatermark: deletionWatermark(plan.JournalWatermark),
		Generation:       plan.Generation,
		ProviderTruthAt:  plan.ProviderTruthAt,
		Accounts:         cloneSlice(plan.Accounts),
	}
	computed, err := CanonicalContractRecoveryPlanHash(contractPlan)
	if err != nil || string(computed) != arm.RecoveryContractPlanHash {
		return fmt.Errorf("%w: contract plan hash mismatch", ErrRecoveryEnvelopeInvalid)
	}
	if err := contractPlan.Validate(); err != nil {
		return fmt.Errorf("%w: contract plan invalid: %v", ErrRecoveryEnvelopeInvalid, err)
	}
	return nil
}

func validateFrozenArm(in RecoveryEnvelopeV1) error {
	arm := in.Frozen
	if arm == nil || arm.Dispositions == nil || len(arm.Dispositions) != len(in.SnapshotAccountInventory) || len(arm.Dispositions) > maxEnvelopeDispositions {
		return fmt.Errorf("%w: frozen dispositions must cover complete snapshot inventory", ErrRecoveryEnvelopeInvalid)
	}
	for i, item := range arm.Dispositions {
		if !validSnapshotAccountID(item.AccountID) || i > 0 && arm.Dispositions[i-1].AccountID >= item.AccountID {
			return fmt.Errorf("%w: frozen dispositions invalid or unsorted", ErrRecoveryEnvelopeInvalid)
		}
		if item.AccountID != in.SnapshotAccountInventory[i].ID {
			return fmt.Errorf("%w: frozen disposition coverage mismatch", ErrRecoveryEnvelopeInvalid)
		}
		if !validFrozenDisposition(item, in.SnapshotAccountInventory[i].Status) {
			return fmt.Errorf("%w: frozen disposition does not match snapshot status", ErrRecoveryEnvelopeInvalid)
		}
	}
	return nil
}

func validFrozenDisposition(item FrozenDispositionV1, status string) bool {
	switch item.Disposition {
	case FrozenDispositionWithheld:
		if status != "active" && status != "restore_pending" {
			return false
		}
		switch item.ReasonCode {
		case "APPROVAL_SCOPE", "PROVIDER_INELIGIBLE", "EVIDENCE_UNAVAILABLE", "AMBIGUOUS", "CURRENT_CHECK_FAILED", "OTHER":
			return true
		}
	case FrozenDispositionDeleted:
		return status == "deleted" && item.ReasonCode == "JOURNAL_DELETED"
	case FrozenDispositionDeleting:
		return status == "deleting" && item.ReasonCode == "JOURNAL_DELETING"
	}
	return false
}

func validateSortedAccountIDs(ids []string, limit int) error {
	if !sortedUniqueStrings(ids, limit, validSnapshotAccountID) {
		return fmt.Errorf("account IDs are missing, invalid, unsorted or over limit")
	}
	return nil
}

func watermarkNoLaterThanPosition(w WatermarkV1, p JournalPositionV1) bool {
	if isZeroWatermark(w) {
		return true
	}
	if w.Generation < p.ActiveGeneration {
		return true
	}
	if w.Generation > p.ActiveGeneration || isZeroWatermark(p.LastObjectInGeneration) {
		return false
	}
	return w.SequenceID <= p.LastObjectInGeneration.SequenceID
}

func deletionWatermark(w WatermarkV1) contracts.DeletionWatermark {
	return contracts.DeletionWatermark{Generation: w.Generation, SequenceID: w.SequenceID, EntryHash: w.EntryHash}
}

func evidenceRefByteLength(r EvidenceRefV1) int {
	return len(r.Authority) + len(r.RecordID) + len(r.Version)
}

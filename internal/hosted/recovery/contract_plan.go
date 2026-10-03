package recovery

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

const maxVerifiedEligibleAccounts = 100

// LegacyPlanArtifactHash names the hash used only by recovery.Plan artifacts.
type LegacyPlanArtifactHash string

// RecoveryContractPlanHash names the separate hash stored in a mapped
// contracts.RecoveryPlan.
type RecoveryContractPlanHash string

// VerifiedEligiblePlan is an opaque proof that an existing immutable legacy
// artifact was loaded and mapped into a separately hashed recovery contract.
// Its fields are intentionally private and it has no arbitrary-plan constructor.
type VerifiedEligiblePlan struct {
	artifact     Plan
	artifactHash LegacyPlanArtifactHash
	contractPlan contracts.RecoveryPlan
	contractHash RecoveryContractPlanHash
}

// createVerifiedEligiblePlan writes through the legacy artifact API, reloads
// the resulting immutable bytes, and only then creates the verified mapping.
func createVerifiedEligiblePlan(ctx context.Context, artifactDir string, input PlanInput) (VerifiedEligiblePlan, error) {
	if err := requireContext(ctx); err != nil {
		return VerifiedEligiblePlan{}, err
	}
	if len(input.Accounts) > maxVerifiedEligibleAccounts {
		return VerifiedEligiblePlan{}, fmt.Errorf("%w: eligible account scope exceeds %d", ErrPlanInvalid, maxVerifiedEligibleAccounts)
	}
	created, err := CreatePlan(ctx, artifactDir, input)
	if err != nil {
		return VerifiedEligiblePlan{}, err
	}
	if err := requireContext(ctx); err != nil {
		return VerifiedEligiblePlan{}, err
	}
	artifact, err := LoadPlan(ctx, artifactDir, created.PlanHash)
	if err != nil {
		return VerifiedEligiblePlan{}, err
	}
	if !plansEqual(created, artifact) {
		return VerifiedEligiblePlan{}, fmt.Errorf("%w: created and reloaded plan artifacts differ", ErrPlanInvalid)
	}
	return verifiedEligiblePlanFromLoadedArtifact(ctx, artifact)
}

// loadVerifiedEligiblePlan reopens an already-created artifact by its artifact
// hash and repeats the complete artifact-to-contract verification.
func loadVerifiedEligiblePlan(ctx context.Context, artifactDir string, artifactHash LegacyPlanArtifactHash) (VerifiedEligiblePlan, error) {
	if err := requireContext(ctx); err != nil {
		return VerifiedEligiblePlan{}, err
	}
	artifact, err := LoadPlan(ctx, artifactDir, string(artifactHash))
	if err != nil {
		return VerifiedEligiblePlan{}, err
	}
	if LegacyPlanArtifactHash(artifact.PlanHash) != artifactHash {
		return VerifiedEligiblePlan{}, fmt.Errorf("%w: legacy artifact hash changed during reload", ErrPlanInvalid)
	}
	return verifiedEligiblePlanFromLoadedArtifact(ctx, artifact)
}

func verifiedEligiblePlanFromLoadedArtifact(ctx context.Context, artifact Plan) (VerifiedEligiblePlan, error) {
	if err := requireContext(ctx); err != nil {
		return VerifiedEligiblePlan{}, err
	}
	if err := validatePlan(artifact); err != nil {
		return VerifiedEligiblePlan{}, err
	}
	if len(artifact.Accounts) == 0 || len(artifact.Accounts) > maxVerifiedEligibleAccounts {
		return VerifiedEligiblePlan{}, fmt.Errorf("%w: eligible account scope is empty or exceeds %d", ErrPlanInvalid, maxVerifiedEligibleAccounts)
	}
	observed, err := time.Parse(time.RFC3339Nano, artifact.ProviderObserved)
	if err != nil || observed.IsZero() || observed.Location() != time.UTC || observed.Format(time.RFC3339Nano) != artifact.ProviderObserved {
		return VerifiedEligiblePlan{}, fmt.Errorf("%w: artifact provider observation is not exact canonical UTC", ErrPlanInvalid)
	}
	accounts := append([]string(nil), artifact.Accounts...)
	plan := contracts.RecoveryPlan{
		SourceSnapshot:   artifact.SnapshotSHA256,
		JournalWatermark: artifact.JournalWatermark,
		Generation:       artifact.FenceGeneration,
		ProviderTruthAt:  observed.UTC(),
		Accounts:         accounts,
	}
	contractHash, err := CanonicalContractRecoveryPlanHash(plan)
	if err != nil {
		return VerifiedEligiblePlan{}, err
	}
	plan.PlanHash = string(contractHash)
	if err := plan.Validate(); err != nil {
		return VerifiedEligiblePlan{}, errors.Join(ErrPlanInvalid, err)
	}
	if err := requireContext(ctx); err != nil {
		return VerifiedEligiblePlan{}, err
	}
	return VerifiedEligiblePlan{
		artifact:     artifact,
		artifactHash: LegacyPlanArtifactHash(artifact.PlanHash),
		contractPlan: plan,
		contractHash: contractHash,
	}, nil
}

package recovery

import (
	"context"
	"errors"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

// RecoveryEnvelopeKind selects one inert canonical data arm. It does not
// represent eligibility, approval, readiness, or any other authority.
type RecoveryEnvelopeKind string

const (
	RecoveryEnvelopeEligible   RecoveryEnvelopeKind = "ELIGIBLE"
	RecoveryEnvelopeFrozenOnly RecoveryEnvelopeKind = "FROZEN_ONLY"
)

const (
	FrozenDispositionWithheld = "WITHHELD_FROZEN"
	FrozenDispositionDeleted  = "DELETED"
	FrozenDispositionDeleting = "DELETING"
)

var (
	// ErrRecoveryEnvelopeInvalid identifies malformed or inconsistent values.
	ErrRecoveryEnvelopeInvalid = errors.New("recovery: invalid recovery envelope")
	// ErrRecoveryEnvelopeTooLarge identifies an exceeded v1 size bound.
	ErrRecoveryEnvelopeTooLarge = errors.New("recovery: recovery envelope exceeds limit")
	// ErrRecoveryEnvelopeNonCanonical identifies wire bytes outside canonical JSON.
	ErrRecoveryEnvelopeNonCanonical = errors.New("recovery: noncanonical recovery envelope")
	// ErrRecoveryEnvelopeContext identifies canceled or missing operation context.
	ErrRecoveryEnvelopeContext = errors.New("recovery: recovery envelope context canceled")
)

// RecoveryEnvelopeHash names the v1 canonical envelope integrity digest.
type RecoveryEnvelopeHash string

// WatermarkV1 is the pure envelope projection of a legacy deletion watermark.
type WatermarkV1 struct {
	Generation int64
	SequenceID int64
	EntryHash  string
}

// JournalPositionV1 is a data projection; it is not an authenticated cursor.
type JournalPositionV1 struct {
	ActiveGeneration       int64
	LastObjectInGeneration WatermarkV1
	PredecessorSeal        WatermarkV1
	GenesisIssuanceID      string
	SuccessorAllocationID  string
}

// SnapshotAccountV1 is a pure account/status inventory row.
type SnapshotAccountV1 struct {
	ID     string
	Status string
}

// BrainInventoryV1 binds the number and digest of a brain projection.
type BrainInventoryV1 struct {
	Count           int
	CanonicalDigest string
}

// EvidenceRefV1 is opaque reference data, not evidence verification.
type EvidenceRefV1 struct {
	Authority string
	RecordID  string
	Version   string
}

// WriterV1 contains unverified old-writer identity fields.
type WriterV1 struct {
	Provider       string
	ScopeRef       string
	WriterRef      string
	BootRef        string
	JournalStoreID string
	Generation     int64
}

// ContractPlanV1 is the canonical contract plan projection without PlanHash.
type ContractPlanV1 struct {
	SourceSnapshot   string
	JournalWatermark WatermarkV1
	Generation       int64
	ProviderTruthAt  time.Time
	Accounts         []string
}

// EligibleArmV1 carries separate artifact and contract-plan hash domains.
type EligibleArmV1 struct {
	LegacyPlanArtifactHash   string
	RecoveryContractPlanHash string
	ContractPlan             ContractPlanV1
	EligibleAccountIDs       []string
}

// FrozenArmV1 carries complete snapshot dispositions for FROZEN_ONLY.
type FrozenArmV1 struct {
	Dispositions []FrozenDispositionV1
}

// FrozenDispositionV1 is a bounded data classification for one account.
type FrozenDispositionV1 struct {
	AccountID   string
	Disposition string
	ReasonCode  string
}

// RecoveryEnvelopeV1 contains canonical data only. None of its fields confer
// authority, and its exported slices remain caller-owned until cloned.
type RecoveryEnvelopeV1 struct {
	FormatVersion            int
	Kind                     RecoveryEnvelopeKind
	PlanRef                  string
	SnapshotPinID            string
	ReservationVersion       int64
	ManifestSHA256           string
	ManifestJournalWatermark WatermarkV1
	SourceBuildToken         string
	SourceSchemaVersion      int
	SnapshotAccountInventory []SnapshotAccountV1
	SnapshotBrainInventory   BrainInventoryV1
	ControlDBLength          int64
	VerifiedArtifactCount    int
	DeclaredArtifactBytes    int64
	SnapshotInventorySHA256  string
	PlanApprovalRef          EvidenceRefV1
	PlanApprovalDigest       string
	PlanApprovalNonce        string
	PlanApprovalExpiresAt    time.Time
	ActivationAllowlist      []string
	OperationID              string
	OldWriter                WriterV1
	JournalStoreID           string
	SnapshotCut              JournalPositionV1
	LastSealed               WatermarkV1
	PrefixEvidenceRef        EvidenceRefV1
	Eligible                 *EligibleArmV1
	Frozen                   *FrozenArmV1
}

// SnapshotInventoryV1 supplies full projection rows for exact source totals.
type SnapshotInventoryV1 struct {
	ManifestSHA256        string
	SourceBuildToken      string
	SourceSchemaVersion   int
	Accounts              []SnapshotAccountV1
	Brains                []contracts.BrainArtifact
	ControlDBLength       int64
	VerifiedArtifactCount int
	DeclaredArtifactBytes int64
}

// SnapshotInventorySummaryV1 contains the fields available in envelope wire.
type SnapshotInventorySummaryV1 struct {
	ManifestSHA256        string
	SourceBuildToken      string
	SourceSchemaVersion   int
	Accounts              []SnapshotAccountV1
	ControlDBLength       int64
	BrainCount            int
	BrainInventorySHA256  string
	VerifiedArtifactCount int
	DeclaredArtifactBytes int64
}

// CloneRecoveryEnvelopeV1 returns a deep copy after validating all bounds.
func CloneRecoveryEnvelopeV1(in RecoveryEnvelopeV1) (RecoveryEnvelopeV1, error) {
	if err := ValidateRecoveryEnvelopeV1(context.Background(), in); err != nil {
		return RecoveryEnvelopeV1{}, err
	}
	return cloneEnvelope(in), nil
}

func cloneEnvelope(in RecoveryEnvelopeV1) RecoveryEnvelopeV1 {
	out := in
	out.SnapshotAccountInventory = cloneSlice(in.SnapshotAccountInventory)
	out.ActivationAllowlist = cloneSlice(in.ActivationAllowlist)
	if in.Eligible != nil {
		eligible := *in.Eligible
		eligible.EligibleAccountIDs = cloneSlice(in.Eligible.EligibleAccountIDs)
		eligible.ContractPlan.Accounts = cloneSlice(in.Eligible.ContractPlan.Accounts)
		out.Eligible = &eligible
	}
	if in.Frozen != nil {
		frozen := *in.Frozen
		frozen.Dispositions = cloneSlice(in.Frozen.Dispositions)
		out.Frozen = &frozen
	}
	return out
}

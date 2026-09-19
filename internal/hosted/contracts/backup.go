package contracts

import (
	"errors"
	"time"
)

// Backup manifest v2 (interfaces.md "Backup manifestv2", owner task49).
// FROZEN, except JournalWatermark: that field carries the deletion journal
// position, whose design is the PROPOSED decision 3 in interfaces.md, so its
// type (DeletionWatermark, self-verifying) is PROPOSED with it. Every other
// field is independent of the four bounded decisions; task49 may implement
// against them once rebased on the integrator commit that adds them to
// internal/hosted/backup.
//
// ManifestV2 supersedes backup.Manifest (version 1, internal/hosted/backup).
// Version 1 records only brain IDs and an empty flag; v2 adds artifact
// checksums/lengths, the source revision, the schema version and the
// deletion journal watermark current at manifest creation, so restore
// (task50) can prove journal completeness from the manifest's own recorded
// watermark forward.
type ManifestV2 struct {
	Version   int // 2
	Source    SourceRef
	CreatedAt time.Time
	Brains    []BrainArtifact
	// JournalWatermark is the deletion journal position read from the
	// independent journal (never the local control database) BEFORE the backup
	// began copying data. Recovery replays every journal entry after it;
	// replaying an entry the snapshot already reflects is safe because purge
	// is idempotent, whereas reading the watermark after the copy could skip a
	// deletion the snapshot missed. This ordering, not Gateway.Maintenance,
	// makes a deletion concurrent with the copy safe: lifecycle deletion does
	// not take that lock.
	JournalWatermark DeletionWatermark
}

type SourceRef struct {
	BuildSHA      string
	SchemaVersion int
}

// BrainArtifact names one bundled brain's artifact by its manifest-relative
// path (never an absolute path — interfaces.md "exact object/record format"
// applies equally here), so a checksum mismatch always names a concrete,
// reproducible file.
type BrainArtifact struct {
	ID           string
	RelativePath string // e.g. "<id>.bundle"; empty when Empty is true
	LengthBytes  int64
	SHA256       string
	Empty        bool
}

// Recovery CLI (interfaces.md "Recovery CLI", owner task50, registration
// owner task41/57). The plan/apply shape is FROZEN except the fields coupled to
// the PROPOSED decisions 3 and 4 in interfaces.md: RecoveryPlan.JournalWatermark,
// RecoveryPlan.Generation and RecoveryApplyResult.Fence. The eligibility rule
// Apply enforces (RecoveryApplyResult.Consistent) is PROPOSED and not approved.
//
// Plan is immutable once produced: PlanHash pins the exact source snapshot,
// deletion-journal watermark and provider truth the plan was computed
// against. Apply refuses to run against a plan whose hash does not match
// what the operator approved, and never accepts an "activate-all" request —
// every phase is scoped to one account at a time and is safe to resume.
type RecoveryPlan struct {
	PlanHash         string
	SourceSnapshot   string // manifest v2 source SHA this plan was computed from
	JournalWatermark DeletionWatermark
	// Generation is the journal generation the plan's activation closes with
	// DeletionJournal.Seal; the restored service then writes generation+1.
	Generation      int64
	ProviderTruthAt time.Time
	Accounts        []string // opaque account IDs eligible for this plan
}

type RecoveryPlanRequest struct {
	SnapshotPath string
}

type RecoveryApplyRequest struct {
	PlanHash  string
	AccountID string // exactly one account per Apply call; never "all"
}

type RecoveryApplyResult struct {
	AccountID string
	Unfrozen  bool
	Reason    string // set and non-empty whenever Unfrozen is false
	// Fence is the proof the old writer was fenced. It must be Sufficient
	// whenever Unfrozen is true.
	Fence FenceReceipt
}

// Consistent reports whether the result obeys the activation rule: an account
// is unfrozen only with a sufficient fence receipt, and every refusal names a
// reason. Task50's Apply must never return a result for which this fails.
func (r RecoveryApplyResult) Consistent() error {
	if r.AccountID == "" {
		return errors.New("hosted/contracts: recovery result has no account")
	}
	if r.Unfrozen {
		return r.Fence.Sufficient()
	}
	if r.Reason == "" {
		return errors.New("hosted/contracts: refused recovery result must state a reason")
	}
	return nil
}

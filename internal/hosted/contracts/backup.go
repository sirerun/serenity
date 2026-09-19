package contracts

import "time"

// Backup manifest v2 (interfaces.md "Backup manifestv2", owner task49).
// FROZEN: this shape does not depend on the four bounded decisions still
// under review; task49 may implement against it once rebased on the
// integrator commit that adds it to internal/hosted/backup.
//
// ManifestV2 supersedes backup.Manifest (version 1, internal/hosted/backup).
// Version 1 records only brain IDs and an empty flag; v2 adds artifact
// checksums/lengths, the source revision, the schema version and the
// deletion journal watermark current at manifest creation, so restore
// (task50) can prove journal completeness from the manifest's own recorded
// watermark forward.
type ManifestV2 struct {
	Version          int // 2
	Source           SourceRef
	CreatedAt        time.Time
	Brains           []BrainArtifact
	JournalWatermark string // contracts.DeletionJournal watermark as of CreatedAt
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
// owner task41/57). FROZEN shape; the eligibility decision Apply ultimately
// enforces is the PROPOSED "Recovery activation barrier" answer in
// interfaces.md's proposed-decisions section and is not yet approved.
//
// Plan is immutable once produced: PlanHash pins the exact source snapshot,
// deletion-journal watermark and provider truth the plan was computed
// against. Apply refuses to run against a plan whose hash does not match
// what the operator approved, and never accepts an "activate-all" request —
// every phase is scoped to one account at a time and is safe to resume.
type RecoveryPlan struct {
	PlanHash         string
	SourceSnapshot   string // manifest v2 source SHA this plan was computed from
	JournalWatermark string
	ProviderTruthAt  time.Time
	Accounts         []string // opaque account IDs eligible for this plan
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
}

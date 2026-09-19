package contracts

import (
	"context"
	"errors"
	"time"
)

// Deletion journal (interfaces.md "Deletion journal", owner task48 with
// infrastructure54). PROPOSED: task41's proposed answer to the "independently
// durable deletion journal" bounded decision — versioned S3 object storage
// as the durable substrate, detailed in interfaces.md's proposed-decisions
// section. Not yet chief-architect approved.
//
// The journal records opaque account/brain IDs and deletion intent/outcome
// only — never email, never memory content. Append must durably succeed
// before the corresponding delete is acknowledged to the caller.
type DeletionSubjectType string

const (
	DeletionSubjectAccount DeletionSubjectType = "account"
	DeletionSubjectBrain   DeletionSubjectType = "brain"
)

type DeletionOutcome string

const (
	DeletionIntentRequested DeletionOutcome = "requested"
	DeletionOutcomePurged   DeletionOutcome = "purged"
)

type DeletionEntry struct {
	SubjectType DeletionSubjectType
	SubjectID   string // opaque account or brain ID, never an email address
	Outcome     DeletionOutcome
	RecordedAt  time.Time
}

// DeletionJournal is the independent durable adapter task48 implements.
// ReadThrough returns every entry recorded strictly after watermark, plus the
// new watermark to persist for the next call; recovery/restore (task50) uses
// ReadThrough against a snapshot's own journal watermark to prove
// completeness through the activation barrier — a bare sequence/checksum
// alone does not prove no tail entry was omitted (interfaces.md, task41
// required steps).
type DeletionJournal interface {
	Append(ctx context.Context, entry DeletionEntry) error
	ReadThrough(ctx context.Context, watermark string) (entries []DeletionEntry, newWatermark string, err error)
}

var (
	// ErrDeletionJournalIncomplete means ReadThrough could not prove it
	// returned every entry up to newWatermark (e.g. the object listing was
	// truncated by the provider and the truncation could not be resolved).
	// Callers (task50) must treat this as a hard failure, never a partial
	// pass.
	ErrDeletionJournalIncomplete = errors.New("hosted/contracts: deletion journal read is not proven complete")
)

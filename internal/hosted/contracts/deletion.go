package contracts

import (
	"context"
	"errors"
	"time"
)

// Deletion journal (interfaces.md "Deletion journal", owner task48 with
// infrastructure54). PROPOSED: task41's proposed answer to the "independently
// durable deletion journal" bounded decision. Not yet chief-architect
// approved.
//
// Revision note: an earlier draft keyed journal objects
// `deletion-journal/<subject_type>/<subject_id>/<recorded_at>-<outcome>.json`
// and described the ReadThrough watermark as an S3 ListObjectsV2
// continuation token, "proven complete" by a non-truncated listing. T23.41's
// own review found this unsound: ListObjectsV2 returns keys in
// lexicographic order, which that key format groups by subject_type then
// subject_id (an opaque, effectively random ID) — not by append time. A
// StartAfter/continuation-token watermark only ever returns keys
// lexicographically greater than the marker, so a later-appended entry
// whose subject_id happens to sort before an already-consumed key is
// silently invisible to every future ReadThrough call, forever — not a
// truncation, so ErrDeletionJournalIncomplete's only check (a truncated
// listing) never catches it. "Strong list-after-write consistency"
// guarantees a single from-scratch listing is complete; it says nothing
// about an incrementally advanced marker across writes made after the
// marker moved. Fixed below: entries are addressed by a durable, monotonic
// SequenceID assigned once by the single currently-fenced writer (never by
// the object store), so key order and append order are the same thing by
// construction, and ReadThrough checks for the literal existence of every
// expected sequence number in range rather than trusting listing order at
// all.
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

// DeletionWatermark is a durable, opaque resume point — never an S3
// pagination token. Generation is the writer generation (interfaces.md §4,
// "Restore eligibility") that assigned SequenceID; SequenceID is a
// per-generation monotonic counter assigned by that one currently-fenced
// writer (proposed substrate: a dedicated counter row in the existing
// hosted control DB, the same single-writer SQLite instance every other
// durable sequence in this service already trusts — not a new service). A
// watermark from a generation other than the current one is only valid once
// interfaces.md §4's activation barrier has verified that whole prior
// generation is closed and fully represented.
type DeletionWatermark struct {
	Generation int64
	SequenceID int64
}

type DeletionEntry struct {
	Watermark   DeletionWatermark // this entry's own (Generation, SequenceID); immutable once Appended
	SubjectType DeletionSubjectType
	SubjectID   string // opaque account or brain ID, never an email address
	Outcome     DeletionOutcome
	RecordedAt  time.Time
}

// DeletionJournal is the independent durable adapter task48 implements.
// Append assigns the entry's SequenceID atomically via the control DB's
// counter (the entry passed in carries no watermark; the one returned does)
// and only then durably writes the S3 object keyed by
// `deletion-journal/<generation>/<sequence, fixed-width zero-padded>.json`
// — the fixed-width, sequence-first key format is what makes lexicographic
// key order equal append order, unlike the subject-first format the review
// found broken.
//
// ReadThrough returns every entry recorded strictly after from, plus the
// watermark to persist for the next call. It proves completeness by reading
// the control DB's own current high-water SequenceID for from.Generation
// (never an S3 listing) as the target, then confirming an object exists for
// every SequenceID in (from.SequenceID, target] — rejecting with
// ErrDeletionJournalIncomplete on the first missing one, whether that gap is
// at the tail (a SequenceID the control DB counted but whose object write
// never completed) or in the middle (any other gap). Recovery/restore
// (task50) uses this against a snapshot's own recorded watermark to prove
// completeness through the activation barrier — a bare sequence/checksum
// alone does not prove no tail or middle entry was omitted (interfaces.md,
// task41 required steps).
type DeletionJournal interface {
	Append(ctx context.Context, entry DeletionEntry) (DeletionEntry, error)
	ReadThrough(ctx context.Context, from DeletionWatermark) (entries []DeletionEntry, to DeletionWatermark, err error)
}

var (
	// ErrDeletionJournalIncomplete means ReadThrough found a gap between
	// from and the control DB's own high-water SequenceID -- at the tail or
	// in the middle of the range, never just a truncated listing. Callers
	// (task50) must treat this as a hard failure, never a partial pass.
	ErrDeletionJournalIncomplete = errors.New("hosted/contracts: deletion journal read is not proven complete")
	// ErrDeletionJournalStaleGeneration means from.Generation is not the
	// current writer generation and interfaces.md §4's activation barrier
	// has not yet certified that prior generation closed; ReadThrough must
	// refuse rather than guess.
	ErrDeletionJournalStaleGeneration = errors.New("hosted/contracts: deletion journal watermark generation is not yet certified closed")
)

package contracts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Deletion journal (interfaces.md "Deletion journal", owner task48 with
// infrastructure54).
//
// STATUS: PROPOSED, not frozen. This is task41's proposed answer to the
// "independently durable deletion journal" bounded decision. It is not
// chief-architect approved. The executable specification of every rule below
// is contractstest.RunJournalSuite.
//
// Design in one paragraph. The journal is a hash-chained, generation-scoped,
// gapless sequence of immutable objects in an object store the local service
// database does not control. The object store itself assigns each position:
// a writer claims sequence N+1 with a conditional create (PutIfAbsent), so
// two writers can never both hold a position, and a losing writer learns it
// has been fenced instead of silently forking history. Completeness is proven
// from the journal alone: keys are fixed-width and sequence-first, so key
// order is append order; every object names its predecessor's hash, so a
// missing middle object breaks the chain; and a seal object, written with the
// same conditional create, closes a generation so no cooperating writer can
// append behind the reader. The local control database is never consulted
// for completeness: it is the very thing restore loses.
//
// What this protocol does NOT prove, stated so a reviewer does not assume
// otherwise: it proves the journal is complete relative to what the object
// store holds at seal time. An adversary with delete rights who removes the
// newest entries before the seal is written is bounded by the IAM policy
// (DeleteObject withheld) and bucket versioning (delete markers stay
// visible), not by the protocol. A writer that ignores its own generation's
// seal is stopped by revoking its credentials (FenceReceipt), not by the
// protocol.
//
// The journal records opaque account/brain IDs and deletion intent/outcome
// only, never email or memory content. Append must durably succeed before the
// corresponding delete is acknowledged to the caller.

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

// DeletionWatermark is a durable, self-verifying resume point. Generation and
// SequenceID locate one journal object; EntryHash is the hash of that object's
// stored bytes, so a reader resuming from a snapshot detects history that
// was rewritten under the same coordinates. The zero value means "before the
// first object of generation 1".
type DeletionWatermark struct {
	Generation int64
	SequenceID int64
	EntryHash  string
}

// IsZero reports whether w is the "start of the journal" watermark.
func (w DeletionWatermark) IsZero() bool { return w == DeletionWatermark{} }

type DeletionEntry struct {
	Watermark   DeletionWatermark // assigned by Append; immutable afterwards
	SubjectType DeletionSubjectType
	SubjectID   string // opaque account or brain ID, never an email address
	Outcome     DeletionOutcome
	RecordedAt  time.Time
}

// Validate rejects an entry that could leak personal data into the journal or
// be unreadable later.
func (e DeletionEntry) Validate() error {
	if e.SubjectType != DeletionSubjectAccount && e.SubjectType != DeletionSubjectBrain {
		return fmt.Errorf("%w: subject type %q", ErrDeletionEntryInvalid, e.SubjectType)
	}
	if e.Outcome != DeletionIntentRequested && e.Outcome != DeletionOutcomePurged {
		return fmt.Errorf("%w: outcome %q", ErrDeletionEntryInvalid, e.Outcome)
	}
	if e.SubjectID == "" || strings.ContainsAny(e.SubjectID, "@ \t\r\n/") {
		return fmt.Errorf("%w: subject id must be a non-empty opaque identifier", ErrDeletionEntryInvalid)
	}
	return nil
}

// JournalObjectKind distinguishes a deletion record from a generation seal.
type JournalObjectKind string

const (
	JournalKindEntry JournalObjectKind = "entry"
	JournalKindSeal  JournalObjectKind = "seal"
)

// JournalObject is the exact stored form of one journal object. The object
// layout is part of this contract because the conformance suite, and a future
// audit tool, must be able to read it independently of the writer.
//
// PrevHash is the hash of the predecessor object's stored bytes: empty only
// for (generation 1, sequence 1); for sequence 1 of generation G>1 it is the
// hash of generation G-1's seal object, so a new generation cannot begin until
// its predecessor is sealed.
type JournalObject struct {
	Kind        JournalObjectKind   `json:"kind"`
	Generation  int64               `json:"generation"`
	SequenceID  int64               `json:"sequence_id"`
	PrevHash    string              `json:"prev_hash"`
	WriterID    string              `json:"writer_id"`
	SubjectType DeletionSubjectType `json:"subject_type,omitempty"`
	SubjectID   string              `json:"subject_id,omitempty"`
	Outcome     DeletionOutcome     `json:"outcome,omitempty"`
	RecordedAt  time.Time           `json:"recorded_at"`
}

// Encode returns the deterministic stored bytes of o.
func (o JournalObject) Encode() ([]byte, error) { return json.Marshal(o) }

// HashObject is the chain hash: hex SHA-256 of an object's stored bytes.
func HashObject(stored []byte) string {
	sum := sha256.Sum256(stored)
	return hex.EncodeToString(sum[:])
}

// JournalPrefix and JournalKey fix the key layout. Generation and sequence are
// zero-padded to ten digits so lexicographic key order equals numeric order
// (sequence first within a generation; no subject or timestamp in the key),
// which is what makes a StartAfter listing a sound incremental read.
func JournalPrefix(generation int64) string {
	return fmt.Sprintf("deletion-journal/%010d/", generation)
}

func JournalKey(generation, sequence int64) string {
	return fmt.Sprintf("%s%010d.json", JournalPrefix(generation), sequence)
}

// JournalObjectStore is the only substrate capability the journal needs. A
// production adapter (task48) maps it onto the existing versioned AWS object
// bucket, and an explicit filesystem/in-memory fake maps it locally.
//
// PutIfAbsent MUST be atomic across every writer: exactly one of two
// concurrent calls for a key reports created=true. On AWS this is S3's
// conditional PutObject (If-None-Match: *); that behavior is asserted here
// from the provider's documented conditional-write feature and has NOT been
// verified against the real service in this task. task48 must prove it with a
// real-provider fixture before the adapter is accepted, and if it cannot the
// substrate decision returns to the architecture review (a dedicated
// conditional-write table would be a new service and needs its own approval).
type JournalObjectStore interface {
	PutIfAbsent(ctx context.Context, key string, body []byte) (created bool, err error)
	Get(ctx context.Context, key string) (body []byte, found bool, err error)
	// ListAfter returns up to limit keys under prefix that sort strictly after
	// startAfter ("" means from the beginning), ascending. more reports that
	// keys remain beyond the returned page.
	ListAfter(ctx context.Context, prefix, startAfter string, limit int) (keys []string, more bool, err error)
}

// DeletionRead is a verified read of the journal.
type DeletionRead struct {
	// Entries are the deletion records strictly after the requested
	// watermark, in append order across every generation traversed. Seal
	// objects are not returned as entries.
	Entries []DeletionEntry
	// To is the watermark of the last object verified, and is the value to
	// persist for the next call.
	To DeletionWatermark
	// Sealed reports that To is a seal object: the generation is closed and
	// nothing may legitimately follow it. Recovery may claim tail
	// completeness only from a Sealed read, never from the highest visible
	// sequence.
	Sealed bool
}

// DeletionJournal is the independent durable adapter task48 implements.
//
// Append chooses the next sequence by conditional create and returns the entry
// with its assigned watermark. If the position is already taken by a different
// object the writer has been fenced: it returns ErrDeletionJournalFenced (or
// ErrDeletionJournalSealed when the occupant is the generation's seal) and the
// service must stop acknowledging deletions. A retry of the very same append
// after a lost response is recognised by byte-identical content and succeeds.
//
// ReadThrough verifies every object it returns: contiguous sequence, matching
// chain hashes, and that the object at from is the one from.EntryHash names.
// It traverses sealed generations into their successors. It never trusts the
// local database or object-store listing order beyond the fixed-width key.
//
// Seal closes generation by conditionally creating the seal object at the
// first free position, then verifies nothing exists beyond it. It is
// idempotent and is the journal half of interfaces.md's activation barrier.
type DeletionJournal interface {
	Append(ctx context.Context, entry DeletionEntry) (DeletionEntry, error)
	ReadThrough(ctx context.Context, from DeletionWatermark) (DeletionRead, error)
	Seal(ctx context.Context, generation int64) (DeletionWatermark, error)
}

var (
	// ErrDeletionEntryInvalid: the entry is malformed or could carry personal data.
	ErrDeletionEntryInvalid = errors.New("hosted/contracts: invalid deletion journal entry")
	// ErrDeletionJournalIncomplete: a gap in the sequence, a broken hash chain,
	// or a generation that began without its predecessor sealed. Recovery must
	// treat it as a hard failure, never a partial pass.
	ErrDeletionJournalIncomplete = errors.New("hosted/contracts: deletion journal read is not proven complete")
	// ErrDeletionJournalHistoryMismatch: the object at the resume watermark is
	// not the one the watermark's hash names, so history was rewritten or the
	// watermark belongs to a different journal.
	ErrDeletionJournalHistoryMismatch = errors.New("hosted/contracts: deletion journal history does not match the watermark")
	// ErrDeletionJournalFenced: another writer holds the position this writer
	// needed. Split brain; the writer must stop acknowledging deletions.
	ErrDeletionJournalFenced = errors.New("hosted/contracts: deletion journal writer is fenced by another writer")
	// ErrDeletionJournalSealed: the generation is closed; this writer is a
	// stale (old) writer and must not append.
	ErrDeletionJournalSealed = errors.New("hosted/contracts: deletion journal generation is sealed")
	// ErrDeletionJournalFenceViolated: an object exists beyond a generation's
	// seal, so some writer ignored the seal. Only credential revocation can
	// contain it; the activation barrier must not open.
	ErrDeletionJournalFenceViolated = errors.New("hosted/contracts: an object exists beyond the deletion journal seal")
)

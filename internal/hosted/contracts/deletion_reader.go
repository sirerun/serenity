package contracts

import "context"

// JournalObjectReader exposes only immutable journal reads. Implementations
// must bound transport allocations before returning object bytes.
type JournalObjectReader interface {
	Get(ctx context.Context, key string) (body []byte, found bool, err error)
	ListAfter(ctx context.Context, prefix, startAfter string, limit int) (keys []string, more bool, err error)
}

// JournalWriterUse records the first verified object authored by an issuance.
type JournalWriterUse struct {
	Generation int64
	WriterID   string
}

// JournalPosition is the authority-selected logical cursor. Its identities
// are inputs to a later authority reconciliation; this package does not
// authenticate them.
type JournalPosition struct {
	ActiveGeneration       int64
	LastObjectInGeneration DeletionWatermark
	PredecessorSeal        DeletionWatermark
	GenesisIssuanceID      string
	SuccessorAllocationID  string
}

// JournalObservation certifies the complete visible canonical history under
// an externally established stable namespace reservation.
type JournalObservation struct {
	Read          DeletionRead
	Position      JournalPosition
	HistorySHA256 string
	WriterUses    []JournalWriterUse
}

// DeletionJournalReader deliberately has no write capability.
type DeletionJournalReader interface {
	Observe(ctx context.Context) (JournalObservation, error)
}

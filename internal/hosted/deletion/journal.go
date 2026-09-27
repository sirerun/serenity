package deletion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

// Journal is the reference contracts.DeletionJournal over a
// contracts.JournalObjectStore. It consults no database: completeness comes
// from the object layout alone.
type Journal struct {
	store    contracts.JournalObjectStore
	writerID string
	gen      int64
	now      func() time.Time

	mu      sync.Mutex
	loaded  bool
	headSeq int64
	head    string
}

func NewJournal(store contracts.JournalObjectStore, writerID string, generation int64, now func() time.Time) *Journal {
	if now == nil {
		now = time.Now
	}
	return &Journal{store: store, writerID: writerID, gen: generation, now: now}
}

// ProductionJournal is the JournalFactory for Journal.
func ProductionJournal(store contracts.JournalObjectStore, writerID string, generation int64, now func() time.Time) contracts.DeletionJournal {
	return NewJournal(store, writerID, generation, now)
}

type stored struct {
	obj  contracts.JournalObject
	hash string
}

// genesisPrev is the PrevHash the first object of generation gen must carry:
// empty for generation 1, otherwise the hash of generation gen-1's seal. A
// generation cannot begin until its predecessor is sealed.
func (j *Journal) genesisPrev(ctx context.Context, gen int64) (string, error) {
	if gen <= 1 {
		return "", nil
	}
	prev, err := j.genesisPrev(ctx, gen-1)
	if err != nil {
		return "", err
	}
	objs, err := j.readGen(ctx, gen-1, 0, prev)
	if err != nil {
		return "", err
	}
	if len(objs) == 0 || objs[len(objs)-1].obj.Kind != contracts.JournalKindSeal {
		return "", fmt.Errorf("%w: generation %d is not sealed", contracts.ErrDeletionJournalIncomplete, gen-1)
	}
	return objs[len(objs)-1].hash, nil
}

// readGen lists generation gen strictly after afterSeq and verifies every
// object: key matches (generation, sequence), sequence is contiguous, PrevHash
// chains from prev, and nothing follows a seal.
func (j *Journal) readGen(ctx context.Context, gen, afterSeq int64, prev string) ([]stored, error) {
	startAfter := ""
	if afterSeq > 0 {
		startAfter = contracts.JournalKey(gen, afterSeq)
	}
	var out []stored
	seq := afterSeq
	sealed := false
	for {
		keys, more, err := j.store.ListAfter(ctx, contracts.JournalPrefix(gen), startAfter, 50)
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			if sealed {
				return nil, contracts.ErrDeletionJournalFenceViolated
			}
			seq++
			if key != contracts.JournalKey(gen, seq) {
				return nil, fmt.Errorf("%w: expected sequence %d in generation %d, found %s", contracts.ErrDeletionJournalIncomplete, seq, gen, key)
			}
			body, found, err := j.store.Get(ctx, key)
			if err != nil {
				return nil, err
			}
			if !found {
				return nil, fmt.Errorf("%w: %s vanished", contracts.ErrDeletionJournalIncomplete, key)
			}
			var obj contracts.JournalObject
			if err := json.Unmarshal(body, &obj); err != nil || obj.Generation != gen || obj.SequenceID != seq || obj.PrevHash != prev {
				return nil, fmt.Errorf("%w: object %s breaks the chain", contracts.ErrDeletionJournalIncomplete, key)
			}
			prev = contracts.HashObject(body)
			out = append(out, stored{obj, prev})
			sealed = obj.Kind == contracts.JournalKindSeal
			startAfter = key
		}
		if !more {
			return out, nil
		}
	}
}

func (j *Journal) loadHead(ctx context.Context) error {
	prev, err := j.genesisPrev(ctx, j.gen)
	if err != nil {
		return err
	}
	objs, err := j.readGen(ctx, j.gen, 0, prev)
	if err != nil {
		return err
	}
	j.headSeq, j.head = 0, prev
	if n := len(objs); n > 0 {
		if objs[n-1].obj.Kind == contracts.JournalKindSeal {
			return contracts.ErrDeletionJournalSealed
		}
		j.headSeq, j.head = int64(n), objs[n-1].hash
	}
	j.loaded = true
	return nil
}

func (j *Journal) AppendDeletion(ctx context.Context, e contracts.DeletionEntry) (contracts.DeletionEntry, error) {
	if err := e.Validate(); err != nil {
		return contracts.DeletionEntry{}, err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if !j.loaded {
		if err := j.loadHead(ctx); err != nil {
			return contracts.DeletionEntry{}, err
		}
	}
	if e.RecordedAt.IsZero() {
		e.RecordedAt = j.now()
	}
	e.RecordedAt = e.RecordedAt.UTC()
	seq := j.headSeq + 1
	body, err := contracts.JournalObject{
		Kind: contracts.JournalKindEntry, Generation: j.gen, SequenceID: seq, PrevHash: j.head, WriterID: j.writerID,
		SubjectType: e.SubjectType, SubjectID: e.SubjectID, Outcome: e.Outcome, RecordedAt: e.RecordedAt,
	}.Encode()
	if err != nil {
		return contracts.DeletionEntry{}, err
	}
	key := contracts.JournalKey(j.gen, seq)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		created, err := j.store.PutIfAbsent(ctx, key, body)
		if err != nil {
			lastErr = err // ambiguous: the object may have landed; retry with the same bytes
			continue
		}
		if !created {
			existing, found, gerr := j.store.Get(ctx, key)
			if gerr != nil {
				lastErr = gerr
				continue
			}
			if found && bytes.Equal(existing, body) {
				created = true // our own earlier put landed before its response was lost
			} else {
				j.loaded = false
				var obj contracts.JournalObject
				if json.Unmarshal(existing, &obj) == nil && obj.Kind == contracts.JournalKindSeal {
					return contracts.DeletionEntry{}, contracts.ErrDeletionJournalSealed
				}
				return contracts.DeletionEntry{}, contracts.ErrDeletionJournalFenced
			}
		}
		if created {
			j.headSeq, j.head = seq, contracts.HashObject(body)
			e.Watermark = contracts.DeletionWatermark{Generation: j.gen, SequenceID: seq, EntryHash: j.head}
			return e, nil
		}
	}
	j.loaded = false
	return contracts.DeletionEntry{}, lastErr
}

func (j *Journal) ReadThrough(ctx context.Context, from contracts.DeletionWatermark) (contracts.DeletionRead, error) {
	gen, afterSeq, prev := int64(1), int64(0), ""
	lastSeal := false
	if !from.IsZero() {
		body, found, err := j.store.Get(ctx, contracts.JournalKey(from.Generation, from.SequenceID))
		if err != nil {
			return contracts.DeletionRead{}, err
		}
		if !found || contracts.HashObject(body) != from.EntryHash {
			return contracts.DeletionRead{}, contracts.ErrDeletionJournalHistoryMismatch
		}
		var obj contracts.JournalObject
		if err := json.Unmarshal(body, &obj); err != nil {
			return contracts.DeletionRead{}, contracts.ErrDeletionJournalHistoryMismatch
		}
		gen, afterSeq, prev, lastSeal = from.Generation, from.SequenceID, from.EntryHash, obj.Kind == contracts.JournalKindSeal
	}
	if from.IsZero() {
		var err error
		if prev, err = j.genesisPrev(ctx, 1); err != nil {
			return contracts.DeletionRead{}, err
		}
	}
	res := contracts.DeletionRead{To: from, Sealed: lastSeal}
	for {
		objs, err := j.readGen(ctx, gen, afterSeq, prev)
		if err != nil {
			return contracts.DeletionRead{}, err
		}
		for _, s := range objs {
			res.To = contracts.DeletionWatermark{Generation: gen, SequenceID: s.obj.SequenceID, EntryHash: s.hash}
			res.Sealed = s.obj.Kind == contracts.JournalKindSeal
			if s.obj.Kind == contracts.JournalKindEntry {
				res.Entries = append(res.Entries, contracts.DeletionEntry{
					Watermark: res.To, SubjectType: s.obj.SubjectType, SubjectID: s.obj.SubjectID, Outcome: s.obj.Outcome, RecordedAt: s.obj.RecordedAt,
				})
			}
		}
		if !res.Sealed {
			// A generation that began without its predecessor sealed is a fence failure.
			next, _, err := j.store.ListAfter(ctx, contracts.JournalPrefix(gen+1), "", 1)
			if err != nil {
				return contracts.DeletionRead{}, err
			}
			if len(next) > 0 {
				return contracts.DeletionRead{}, fmt.Errorf("%w: generation %d has objects but generation %d is not sealed", contracts.ErrDeletionJournalIncomplete, gen+1, gen)
			}
			return res, nil
		}
		// Sealed: continue into the successor generation if it has begun.
		nextObjs, err := j.readGen(ctx, gen+1, 0, res.To.EntryHash)
		if err != nil {
			return contracts.DeletionRead{}, err
		}
		if len(nextObjs) == 0 {
			return res, nil
		}
		gen, afterSeq, prev = gen+1, 0, res.To.EntryHash
		res.Sealed = false
	}
}

func (j *Journal) Seal(ctx context.Context, generation int64) (contracts.DeletionWatermark, error) {
	prev, err := j.genesisPrev(ctx, generation)
	if err != nil {
		return contracts.DeletionWatermark{}, err
	}
	// Verify the whole generation once; a retry after losing a position to a
	// live writer re-reads only the suffix appended since, so a busy writer
	// cannot make each attempt slower than the writer's own append.
	objs, err := j.readGen(ctx, generation, 0, prev)
	if err != nil {
		return contracts.DeletionWatermark{}, err
	}
	head, headHash := int64(len(objs)), prev
	if head > 0 {
		headHash = objs[head-1].hash
	}
	for attempt := 0; attempt < 64; attempt++ {
		if head > 0 && objs[head-1].obj.Kind == contracts.JournalKindSeal {
			return contracts.DeletionWatermark{Generation: generation, SequenceID: head, EntryHash: headHash}, nil
		}
		body, err := contracts.JournalObject{
			Kind: contracts.JournalKindSeal, Generation: generation, SequenceID: head + 1, PrevHash: headHash,
			WriterID: j.writerID, RecordedAt: j.now().UTC(),
		}.Encode()
		if err != nil {
			return contracts.DeletionWatermark{}, err
		}
		key := contracts.JournalKey(generation, head+1)
		created, err := j.store.PutIfAbsent(ctx, key, body)
		if err != nil {
			return contracts.DeletionWatermark{}, err
		}
		if created {
			beyond, _, err := j.store.ListAfter(ctx, contracts.JournalPrefix(generation), key, 1)
			if err != nil {
				return contracts.DeletionWatermark{}, err
			}
			if len(beyond) > 0 {
				return contracts.DeletionWatermark{}, contracts.ErrDeletionJournalFenceViolated
			}
			if generation == j.gen {
				j.mu.Lock()
				j.loaded = false
				j.mu.Unlock()
			}
			return contracts.DeletionWatermark{Generation: generation, SequenceID: head + 1, EntryHash: contracts.HashObject(body)}, nil
		}
		// A live writer took the position first: read what it appended and try the next one.
		more, err := j.readGen(ctx, generation, head, headHash)
		if err != nil {
			return contracts.DeletionWatermark{}, err
		}
		objs = append(objs, more...)
		head = int64(len(objs))
		if len(more) > 0 {
			headHash = more[len(more)-1].hash
		}
	}
	return contracts.DeletionWatermark{}, contracts.ErrDeletionJournalFenced
}

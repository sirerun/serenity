package deletion

import (
	"bytes"
	"context"
	"fmt"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

func (j *Journal) prepareReserved(ctx context.Context) error {
	if j.prepared {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	reader, err := NewJournalReader(j.reader, *j.reserved)
	if err != nil {
		return err
	}
	observation, err := reader.Observe(ctx)
	if err != nil {
		return err
	}
	if observation.Position != *j.reserved {
		return contracts.ErrDeletionJournalHistoryMismatch
	}
	for _, use := range observation.WriterUses {
		if use.WriterID == j.writerID && use.Generation != j.gen {
			return fmt.Errorf("journal writer identity was already used in another generation")
		}
	}
	j.prepared = true
	return nil
}

func sameAppendRequest(p *pendingObject, entry contracts.DeletionEntry) bool {
	if p.kind != contracts.JournalKindEntry || p.entry.SubjectType != entry.SubjectType || p.entry.SubjectID != entry.SubjectID || p.entry.Outcome != entry.Outcome {
		return false
	}
	return entry.RecordedAt.IsZero() || entry.RecordedAt.UTC().Equal(p.entry.RecordedAt)
}

func (j *Journal) appendReserved(ctx context.Context, entry contracts.DeletionEntry) (contracts.DeletionEntry, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if ctx == nil {
		return contracts.DeletionEntry{}, fmt.Errorf("nil journal context")
	}
	if err := j.prepareReserved(ctx); err != nil {
		return contracts.DeletionEntry{}, err
	}
	if j.sealedAt != nil {
		return contracts.DeletionEntry{}, contracts.ErrDeletionJournalSealed
	}
	if j.pending != nil {
		if !sameAppendRequest(j.pending, entry) {
			return contracts.DeletionEntry{}, fmt.Errorf("journal has an unresolved different write")
		}
		entry = j.pending.entry
	} else {
		if entry.RecordedAt.IsZero() {
			entry.RecordedAt = j.now()
		}
		entry.RecordedAt = entry.RecordedAt.UTC()
		if entry.RecordedAt.IsZero() {
			return contracts.DeletionEntry{}, fmt.Errorf("journal clock returned zero time")
		}
		sequence := j.headSeq + 1
		body, err := (contracts.JournalObject{Kind: contracts.JournalKindEntry, Generation: j.gen, SequenceID: sequence, PrevHash: j.head, WriterID: j.writerID, SubjectType: entry.SubjectType, SubjectID: entry.SubjectID, Outcome: entry.Outcome, RecordedAt: entry.RecordedAt}).Encode()
		if err != nil {
			return contracts.DeletionEntry{}, err
		}
		j.pending = &pendingObject{key: contracts.JournalKey(j.gen, sequence), body: body, sequence: sequence, kind: contracts.JournalKindEntry, entry: entry}
	}
	wm, err := j.commitReservedPending(ctx)
	if err != nil {
		return contracts.DeletionEntry{}, err
	}
	entry.Watermark = wm
	return entry, nil
}

func (j *Journal) commitReservedPending(ctx context.Context) (contracts.DeletionWatermark, error) {
	pending := j.pending
	if pending == nil {
		return contracts.DeletionWatermark{}, fmt.Errorf("no pending journal object")
	}
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return contracts.DeletionWatermark{}, err
		}
		created, err := j.store.PutIfAbsent(ctx, pending.key, pending.body)
		if err == nil && created {
			return j.finishPending(pending), nil
		}
		if err != nil {
			lastErr = err
		}
		body, found, getErr := j.store.Get(ctx, pending.key)
		if getErr != nil {
			lastErr = getErr
			continue
		}
		if found {
			if bytes.Equal(body, pending.body) {
				return j.finishPending(pending), nil
			}
			return contracts.DeletionWatermark{}, contracts.ErrDeletionJournalFenced
		}
		if err == nil && !created {
			return contracts.DeletionWatermark{}, contracts.ErrDeletionJournalFenced
		}
	}
	if lastErr == nil {
		lastErr = contracts.ErrDeletionJournalFenced
	}
	return contracts.DeletionWatermark{}, lastErr
}

func (j *Journal) finishPending(pending *pendingObject) contracts.DeletionWatermark {
	watermark := contracts.DeletionWatermark{Generation: j.gen, SequenceID: pending.sequence, EntryHash: contracts.HashObject(pending.body)}
	j.headSeq, j.head = pending.sequence, watermark.EntryHash
	if pending.kind == contracts.JournalKindSeal {
		j.sealedAt = &watermark
		j.sealedBytes = append([]byte(nil), pending.body...)
	}
	j.pending = nil
	return watermark
}

func (j *Journal) sealReserved(ctx context.Context, generation int64) (contracts.DeletionWatermark, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if ctx == nil {
		return contracts.DeletionWatermark{}, fmt.Errorf("nil journal context")
	}
	if generation != j.gen {
		return contracts.DeletionWatermark{}, fmt.Errorf("reserved journal cannot seal generation %d", generation)
	}
	if err := j.prepareReserved(ctx); err != nil {
		return contracts.DeletionWatermark{}, err
	}
	if j.sealedAt != nil {
		if err := j.verifyOwnSeal(ctx, *j.sealedAt); err != nil {
			return contracts.DeletionWatermark{}, err
		}
		return *j.sealedAt, nil
	}
	if j.pending != nil && j.pending.kind != contracts.JournalKindSeal {
		return contracts.DeletionWatermark{}, fmt.Errorf("journal has an unresolved different write")
	}
	if j.pending == nil {
		sequence := j.headSeq + 1
		recordedAt := j.now().UTC()
		if recordedAt.IsZero() {
			return contracts.DeletionWatermark{}, fmt.Errorf("journal clock returned zero time")
		}
		body, err := (contracts.JournalObject{Kind: contracts.JournalKindSeal, Generation: j.gen, SequenceID: sequence, PrevHash: j.head, WriterID: j.writerID, RecordedAt: recordedAt}).Encode()
		if err != nil {
			return contracts.DeletionWatermark{}, err
		}
		j.pending = &pendingObject{key: contracts.JournalKey(j.gen, sequence), body: body, sequence: sequence, kind: contracts.JournalKindSeal}
	}
	watermark, err := j.commitReservedPending(ctx)
	if err != nil {
		return contracts.DeletionWatermark{}, err
	}
	if err := j.verifyOwnSeal(ctx, watermark); err != nil {
		return contracts.DeletionWatermark{}, err
	}
	return watermark, nil
}

func (j *Journal) verifyOwnSeal(ctx context.Context, watermark contracts.DeletionWatermark) error {
	key := contracts.JournalKey(watermark.Generation, watermark.SequenceID)
	body, found, err := j.store.Get(ctx, key)
	if err != nil {
		return err
	}
	if !found || contracts.HashObject(body) != watermark.EntryHash || !bytes.Equal(body, j.sealedBytes) {
		return contracts.ErrDeletionJournalHistoryMismatch
	}
	obj, err := decodeJournalObject(body)
	if err != nil || obj.Kind != contracts.JournalKindSeal || obj.WriterID != j.writerID {
		return contracts.ErrDeletionJournalHistoryMismatch
	}
	keys, more, err := j.store.ListAfter(ctx, contracts.JournalPrefix(j.gen), key, 1)
	if err != nil {
		return err
	}
	if len(keys) != 0 || more {
		return contracts.ErrDeletionJournalFenceViolated
	}
	return nil
}

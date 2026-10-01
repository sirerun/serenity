package deletion

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

type markerOverlayStore struct {
	contracts.JournalObjectStore
	markerKey string
}

func (s *markerOverlayStore) ListAfter(ctx context.Context, prefix, startAfter string, limit int) ([]string, bool, error) {
	keys, more, err := s.JournalObjectStore.ListAfter(ctx, prefix, startAfter, limit)
	if err != nil || s.markerKey == "" || !strings.HasPrefix(s.markerKey, prefix) || s.markerKey <= startAfter {
		return keys, more, err
	}
	keys = append(keys, s.markerKey)
	sort.Strings(keys)
	unique := keys[:0]
	for _, key := range keys {
		if len(unique) == 0 || unique[len(unique)-1] != key {
			unique = append(unique, key)
		}
	}
	if len(unique) > limit {
		return unique[:limit], true, nil
	}
	return unique, more, nil
}

func appendAndSealGenerationOne(t *testing.T, store contracts.JournalObjectStore) contracts.DeletionWatermark {
	t.Helper()
	writer := NewJournal(store, "writer-1", 1, nil)
	if _, err := writer.AppendDeletion(context.Background(), contracts.DeletionEntry{
		SubjectType: contracts.DeletionSubjectAccount,
		SubjectID:   "account-1234567890",
		Outcome:     contracts.DeletionIntentRequested,
	}); err != nil {
		t.Fatal(err)
	}
	seal, err := writer.Seal(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	return seal
}

func TestReadThroughFromSealRejectsAnySameGenerationTail(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, store *FileObjectStore, seal contracts.DeletionWatermark) contracts.DeletionJournal
	}{
		{
			name: "valid entry",
			setup: func(t *testing.T, store *FileObjectStore, seal contracts.DeletionWatermark) contracts.DeletionJournal {
				putJournalObject(t, store, 1, seal.SequenceID+1, contracts.JournalObject{
					Kind: contracts.JournalKindEntry, Generation: 1, SequenceID: seal.SequenceID + 1,
					PrevHash: seal.EntryHash, WriterID: "stale-writer",
					SubjectType: contracts.DeletionSubjectAccount, SubjectID: "account-2234567890",
					Outcome: contracts.DeletionIntentRequested, RecordedAt: validJournalEntryObject(1, 1).RecordedAt,
				})
				return NewJournal(store, "reader", 1, nil)
			},
		},
		{
			name: "malformed object",
			setup: func(t *testing.T, store *FileObjectStore, seal contracts.DeletionWatermark) contracts.DeletionJournal {
				key := contracts.JournalKey(1, seal.SequenceID+1)
				created, err := store.PutIfAbsent(context.Background(), key, []byte("{"))
				if err != nil || !created {
					t.Fatalf("put malformed tail: created=%v err=%v", created, err)
				}
				return NewJournal(store, "reader", 1, nil)
			},
		},
		{
			name: "delete marker without a current body",
			setup: func(t *testing.T, store *FileObjectStore, seal contracts.DeletionWatermark) contracts.DeletionJournal {
				key := contracts.JournalKey(1, seal.SequenceID+1)
				return NewJournal(&markerOverlayStore{JournalObjectStore: store, markerKey: key}, "reader", 1, nil)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewFileObjectStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			seal := appendAndSealGenerationOne(t, store)
			reader := tt.setup(t, store, seal)
			if _, err = reader.ReadThrough(context.Background(), seal); !errors.Is(err, contracts.ErrDeletionJournalFenceViolated) {
				t.Fatalf("ReadThrough from seal err = %v, want ErrDeletionJournalFenceViolated", err)
			}
		})
	}
}

func TestReadThroughRejectsConfiguredGenerationGap(t *testing.T) {
	store, err := NewFileObjectStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seal := appendAndSealGenerationOne(t, store)
	putJournalObject(t, store, 3, 1, contracts.JournalObject{
		Kind: contracts.JournalKindEntry, Generation: 3, SequenceID: 1,
		PrevHash: seal.EntryHash, WriterID: "writer-3",
		SubjectType: contracts.DeletionSubjectAccount, SubjectID: "account-3234567890",
		Outcome: contracts.DeletionIntentRequested, RecordedAt: validJournalEntryObject(1, 1).RecordedAt,
	})

	journal := NewJournal(store, "reader", 3, nil)
	for _, tt := range []struct {
		name string
		from contracts.DeletionWatermark
	}{{name: "from beginning"}, {name: "from generation-one seal", from: seal}} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err = journal.ReadThrough(context.Background(), tt.from); !errors.Is(err, contracts.ErrDeletionJournalIncomplete) {
				t.Fatalf("ReadThrough from %+v err = %v, want ErrDeletionJournalIncomplete", tt.from, err)
			}
		})
	}
}

func TestReadThroughAllowsValidSuccessorAboveConfiguredGeneration(t *testing.T) {
	store, err := NewFileObjectStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	g1 := NewJournal(store, "writer-1", 1, nil)
	if _, err = g1.AppendDeletion(context.Background(), contracts.DeletionEntry{SubjectType: contracts.DeletionSubjectAccount, SubjectID: "account-1234567890", Outcome: contracts.DeletionIntentRequested}); err != nil {
		t.Fatal(err)
	}
	if _, err = g1.Seal(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	g2 := NewJournal(store, "writer-2", 2, nil)
	if _, err = g2.AppendDeletion(context.Background(), contracts.DeletionEntry{SubjectType: contracts.DeletionSubjectAccount, SubjectID: "account-2234567890", Outcome: contracts.DeletionIntentRequested}); err != nil {
		t.Fatal(err)
	}
	if _, err = g2.Seal(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	g3 := NewJournal(store, "writer-3", 3, nil)
	if _, err = g3.AppendDeletion(context.Background(), contracts.DeletionEntry{SubjectType: contracts.DeletionSubjectAccount, SubjectID: "account-3234567890", Outcome: contracts.DeletionIntentRequested}); err != nil {
		t.Fatal(err)
	}

	reader := NewJournal(store, "reader", 2, nil)
	got, err := reader.ReadThrough(context.Background(), contracts.DeletionWatermark{})
	if err != nil || len(got.Entries) != 3 || got.To.Generation != 3 || got.Sealed {
		t.Fatalf("ReadThrough = %+v, %v; want complete unsealed generation 3 successor", got, err)
	}
}

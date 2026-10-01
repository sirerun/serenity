package deletion

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

func putJournalObject(t *testing.T, store contracts.JournalObjectStore, generation, sequence int64, object contracts.JournalObject) contracts.DeletionWatermark {
	t.Helper()
	body, err := object.Encode()
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.PutIfAbsent(context.Background(), contracts.JournalKey(generation, sequence), body)
	if err != nil || !created {
		t.Fatalf("put journal object: created=%v err=%v", created, err)
	}
	return contracts.DeletionWatermark{Generation: generation, SequenceID: sequence, EntryHash: contracts.HashObject(body)}
}

func validJournalEntryObject(generation, sequence int64) contracts.JournalObject {
	return contracts.JournalObject{
		Kind: contracts.JournalKindEntry, Generation: generation, SequenceID: sequence,
		WriterID: "writer-1", SubjectType: contracts.DeletionSubjectBrain,
		SubjectID: "brain-1234567890", Outcome: contracts.DeletionIntentRequested,
		RecordedAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC),
	}
}

func TestReadThroughRejectsMalformedJournalRecord(t *testing.T) {
	tests := []struct {
		name   string
		object contracts.JournalObject
	}{
		{
			name: "unknown kind",
			object: func() contracts.JournalObject {
				obj := validJournalEntryObject(1, 1)
				obj.Kind = "future-or-corrupt-kind"
				return obj
			}(),
		},
		{
			name: "invalid entry payload",
			object: func() contracts.JournalObject {
				obj := validJournalEntryObject(1, 1)
				obj.SubjectID = ""
				return obj
			}(),
		},
		{
			name: "seal carries entry fields",
			object: func() contracts.JournalObject {
				obj := validJournalEntryObject(1, 1)
				obj.Kind = contracts.JournalKindSeal
				return obj
			}(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewFileObjectStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			putJournalObject(t, store, 1, 1, tt.object)
			journal := NewJournal(store, "reader", 1, nil)
			if _, err = journal.ReadThrough(context.Background(), contracts.DeletionWatermark{}); !errors.Is(err, contracts.ErrDeletionJournalIncomplete) {
				t.Fatalf("ReadThrough err = %v, want ErrDeletionJournalIncomplete", err)
			}
		})
	}
}

func TestReadThroughRejectsMalformedResumeObject(t *testing.T) {
	tests := []struct {
		name      string
		object    contracts.JournalObject
		watermark contracts.DeletionWatermark
		wantErr   error
	}{
		{
			name:      "coordinate mismatch",
			object:    validJournalEntryObject(2, 1),
			watermark: contracts.DeletionWatermark{Generation: 1, SequenceID: 1},
			wantErr:   contracts.ErrDeletionJournalHistoryMismatch,
		},
		{
			name: "unknown kind at matching coordinate",
			object: func() contracts.JournalObject {
				obj := validJournalEntryObject(1, 1)
				obj.Kind = "future-or-corrupt-kind"
				return obj
			}(),
			watermark: contracts.DeletionWatermark{Generation: 1, SequenceID: 1},
			wantErr:   contracts.ErrDeletionJournalHistoryMismatch,
		},
		{
			name: "invalid entry payload at matching coordinate",
			object: func() contracts.JournalObject {
				obj := validJournalEntryObject(1, 1)
				obj.SubjectID = ""
				return obj
			}(),
			watermark: contracts.DeletionWatermark{Generation: 1, SequenceID: 1},
			wantErr:   contracts.ErrDeletionJournalHistoryMismatch,
		},
		{
			name: "seal carries entry fields at matching coordinate",
			object: func() contracts.JournalObject {
				obj := validJournalEntryObject(1, 1)
				obj.Kind = contracts.JournalKindSeal
				return obj
			}(),
			watermark: contracts.DeletionWatermark{Generation: 1, SequenceID: 1},
			wantErr:   contracts.ErrDeletionJournalHistoryMismatch,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewFileObjectStore(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			actual := putJournalObject(t, store, 1, 1, tt.object)
			tt.watermark.EntryHash = actual.EntryHash
			journal := NewJournal(store, "reader", 1, nil)
			if _, err = journal.ReadThrough(context.Background(), tt.watermark); !errors.Is(err, tt.wantErr) {
				t.Fatalf("ReadThrough err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

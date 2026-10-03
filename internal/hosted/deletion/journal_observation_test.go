package deletion

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/hosted/contracts"
)

func TestObserveFullHistoryIncludesSealDigestAndEmptySuccessor(t *testing.T) {
	store, err := NewFileObjectStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	clock := func() time.Time { return time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC) }
	first := NewJournal(store, "writer-genesis", 1, clock)
	entry, err := first.AppendDeletion(context.Background(), contracts.DeletionEntry{SubjectType: contracts.DeletionSubjectBrain, SubjectID: "brain-1234567890", Outcome: contracts.DeletionIntentRequested})
	if err != nil {
		t.Fatal(err)
	}
	seal, err := first.Seal(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	position := contracts.JournalPosition{ActiveGeneration: 2, PredecessorSeal: seal, SuccessorAllocationID: "allocation-2"}
	reader, err := NewJournalReader(store, position)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := reader.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if observation.Read.To != seal || !observation.Read.Sealed || len(observation.Read.Entries) != 1 || observation.Read.Entries[0].Watermark != entry.Watermark {
		t.Fatalf("unexpected read projection: %#v", observation.Read)
	}
	if len(observation.WriterUses) != 1 || observation.WriterUses[0] != (contracts.JournalWriterUse{Generation: 1, WriterID: "writer-genesis"}) {
		t.Fatalf("writer uses = %#v", observation.WriterUses)
	}
	var canonical []byte
	for _, sequence := range []int64{1, 2} {
		body, found, err := store.Get(context.Background(), contracts.JournalKey(1, sequence))
		if err != nil || !found {
			t.Fatalf("get history object: found=%v err=%v", found, err)
		}
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(body)))
		canonical = append(canonical, size[:]...)
		canonical = append(canonical, body...)
	}
	want := sha256.Sum256(canonical)
	if observation.HistorySHA256 != hex.EncodeToString(want[:]) {
		t.Fatalf("history digest %s, want %x", observation.HistorySHA256, want)
	}
}

func TestObserveAcceptsGenesisAndUnsealedSuccessorHeads(t *testing.T) {
	t.Run("empty genesis", func(t *testing.T) {
		store, err := NewFileObjectStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		reader, err := NewJournalReader(store, contracts.JournalPosition{ActiveGeneration: 1, GenesisIssuanceID: "genesis-1"})
		if err != nil {
			t.Fatal(err)
		}
		observation, err := reader.Observe(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !observation.Read.To.IsZero() || observation.Read.Sealed || observation.HistorySHA256 != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
			t.Fatalf("empty genesis observation = %#v", observation)
		}
	})
	t.Run("ordinary unsealed successor", func(t *testing.T) {
		store, err := NewFileObjectStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		first := NewJournal(store, "writer-one", 1, nil)
		seal, err := first.Seal(context.Background(), 1)
		if err != nil {
			t.Fatal(err)
		}
		second := NewJournal(store, "writer-two", 2, nil)
		entry, err := second.AppendDeletion(context.Background(), contracts.DeletionEntry{SubjectType: contracts.DeletionSubjectBrain, SubjectID: "brain-1234567890", Outcome: contracts.DeletionIntentRequested})
		if err != nil {
			t.Fatal(err)
		}
		position := contracts.JournalPosition{ActiveGeneration: 2, PredecessorSeal: seal, SuccessorAllocationID: "allocation-2", LastObjectInGeneration: entry.Watermark}
		reader, err := NewJournalReader(store, position)
		if err != nil {
			t.Fatal(err)
		}
		observation, err := reader.Observe(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if observation.Read.To != entry.Watermark || observation.Read.Sealed || len(observation.Read.Entries) != 1 || !reflect.DeepEqual(observation.WriterUses, []contracts.JournalWriterUse{{Generation: 1, WriterID: "writer-one"}, {Generation: 2, WriterID: "writer-two"}}) {
			t.Fatalf("unsealed successor observation = %#v", observation)
		}
	})
}

func TestNewJournalAtRejectsCrossGenerationWriterBeforePut(t *testing.T) {
	base, err := NewFileObjectStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first := NewJournal(base, "reused-id", 1, nil)
	seal, err := first.Seal(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	store := &countingJournalStore{JournalObjectStore: base}
	position := contracts.JournalPosition{ActiveGeneration: 2, PredecessorSeal: seal, SuccessorAllocationID: "allocation-2"}
	journal, err := NewJournalAt(store, "reused-id", position, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = journal.AppendDeletion(context.Background(), contracts.DeletionEntry{SubjectType: contracts.DeletionSubjectBrain, SubjectID: "brain-1234567890", Outcome: contracts.DeletionIntentRequested})
	if err == nil || store.puts != 0 {
		t.Fatalf("append err=%v, conditional puts=%d", err, store.puts)
	}
}

func TestReservedSealIsIdempotentForOwnSeal(t *testing.T) {
	store, err := NewFileObjectStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	journal, err := NewJournalAt(store, "writer-1", contracts.JournalPosition{ActiveGeneration: 1, GenesisIssuanceID: "genesis-1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	first, err := journal.Seal(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := journal.Seal(context.Background(), 1)
	if err != nil || second != first {
		t.Fatalf("seal replay = %#v, %v; first %#v", second, err, first)
	}
}

func TestReservedSealRetryKeepsExactTimestampBytes(t *testing.T) {
	base, err := NewFileObjectStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := &ambiguousJournalStore{JournalObjectStore: base}
	calls := 0
	now := func() time.Time { calls++; return time.Date(2026, 10, 3, 2, 0, calls, 0, time.UTC) }
	journal, err := NewJournalAt(store, "writer-1", contracts.JournalPosition{ActiveGeneration: 1, GenesisIssuanceID: "genesis-1"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Seal(context.Background(), 1); err == nil {
		t.Fatal("ambiguous seal unexpectedly succeeded")
	}
	body := append([]byte(nil), store.body...)
	if calls != 1 {
		t.Fatalf("clock calls after ambiguous seal = %d", calls)
	}
	watermark, err := journal.Seal(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	stored, found, err := base.Get(context.Background(), contracts.JournalKey(1, 1))
	if err != nil || !found || !reflect.DeepEqual(stored, body) || watermark.EntryHash != contracts.HashObject(body) || calls != 1 {
		t.Fatalf("seal retry changed bytes or time: found=%v err=%v clock=%d", found, err, calls)
	}
}

func TestReservedSealRefusesForeignSealAndChangedOwnSeal(t *testing.T) {
	t.Run("foreign active seal", func(t *testing.T) {
		store, err := NewFileObjectStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		foreign := NewJournal(store, "foreign-writer", 1, nil)
		seal, err := foreign.Seal(context.Background(), 1)
		if err != nil {
			t.Fatal(err)
		}
		journal, err := NewJournalAt(store, "new-writer", contracts.JournalPosition{ActiveGeneration: 1, GenesisIssuanceID: "genesis-1", LastObjectInGeneration: seal}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := journal.Seal(context.Background(), 1); !errors.Is(err, contracts.ErrDeletionJournalSealed) {
			t.Fatalf("foreign seal error = %v", err)
		}
	})
	t.Run("changed own seal", func(t *testing.T) {
		base, err := NewFileObjectStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		store := &rewriteJournalGetStore{JournalObjectStore: base}
		journal, err := NewJournalAt(store, "writer-1", contracts.JournalPosition{ActiveGeneration: 1, GenesisIssuanceID: "genesis-1"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		seal, err := journal.Seal(context.Background(), 1)
		if err != nil {
			t.Fatal(err)
		}
		store.key = contracts.JournalKey(1, 1)
		store.rewritten = []byte("changed bytes")
		if _, err := journal.Seal(context.Background(), 1); !errors.Is(err, contracts.ErrDeletionJournalHistoryMismatch) {
			t.Fatalf("changed seal error = %v", err)
		}
		_ = seal
	})
	t.Run("post seal tail", func(t *testing.T) {
		store, err := NewFileObjectStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		journal, err := NewJournalAt(store, "writer-1", contracts.JournalPosition{ActiveGeneration: 1, GenesisIssuanceID: "genesis-1"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		seal, err := journal.Seal(context.Background(), 1)
		if err != nil {
			t.Fatal(err)
		}
		putJournalObject(t, store, 1, 2, contracts.JournalObject{Kind: contracts.JournalKindEntry, Generation: 1, SequenceID: 2, PrevHash: seal.EntryHash, WriterID: "foreign", SubjectType: contracts.DeletionSubjectBrain, SubjectID: "brain-1234567890", Outcome: contracts.DeletionIntentRequested, RecordedAt: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)})
		if _, err := journal.Seal(context.Background(), 1); !errors.Is(err, contracts.ErrDeletionJournalFenceViolated) {
			t.Fatalf("post-seal tail error = %v", err)
		}
	})
}

func TestReservedPendingAppendRetainsExactBytesAfterAmbiguousError(t *testing.T) {
	base, err := NewFileObjectStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := &ambiguousJournalStore{JournalObjectStore: base}
	now := func() time.Time { return time.Date(2026, 10, 3, 1, 2, 3, 0, time.UTC) }
	journal, err := NewJournalAt(store, "writer-1", contracts.JournalPosition{ActiveGeneration: 1, GenesisIssuanceID: "genesis-1"}, now)
	if err != nil {
		t.Fatal(err)
	}
	entry := contracts.DeletionEntry{SubjectType: contracts.DeletionSubjectBrain, SubjectID: "brain-1234567890", Outcome: contracts.DeletionIntentRequested}
	if _, err := journal.AppendDeletion(context.Background(), entry); err == nil {
		t.Fatal("ambiguous injected response unexpectedly succeeded")
	}
	firstBytes := append([]byte(nil), store.body...)
	result, err := journal.AppendDeletion(context.Background(), entry)
	if err != nil {
		t.Fatal(err)
	}
	stored, found, err := base.Get(context.Background(), contracts.JournalKey(1, 1))
	if err != nil || !found || string(stored) != string(firstBytes) || result.Watermark.EntryHash != contracts.HashObject(firstBytes) {
		t.Fatalf("retry changed pending bytes: found=%v err=%v result=%#v", found, err, result)
	}
}

func TestObserveRejectsStaleHeadAndPositionShape(t *testing.T) {
	store, err := NewFileObjectStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	journal := NewJournal(store, "writer-1", 1, nil)
	entry, err := journal.AppendDeletion(context.Background(), contracts.DeletionEntry{SubjectType: contracts.DeletionSubjectBrain, SubjectID: "brain-1234567890", Outcome: contracts.DeletionIntentRequested})
	if err != nil {
		t.Fatal(err)
	}
	wrong := entry.Watermark
	wrong.EntryHash = strings.Repeat("0", 64)
	reader, err := NewJournalReader(store, contracts.JournalPosition{ActiveGeneration: 1, GenesisIssuanceID: "genesis-1", LastObjectInGeneration: wrong})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Observe(context.Background()); !errors.Is(err, contracts.ErrDeletionJournalHistoryMismatch) {
		t.Fatalf("stale head error = %v", err)
	}
	if _, err := NewJournalReader(store, contracts.JournalPosition{ActiveGeneration: 0}); err == nil {
		t.Fatal("generation zero accepted")
	}
	if _, err := NewJournalAt(store, " "+strings.Repeat("x", 300), contracts.JournalPosition{ActiveGeneration: 1, GenesisIssuanceID: "genesis-1"}, nil); err == nil {
		t.Fatal("invalid writer id accepted")
	}
}

func TestObserveRejectsGapAndCrossGenerationWriterReuse(t *testing.T) {
	t.Run("unsealed predecessor", func(t *testing.T) {
		store, err := NewFileObjectStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		putJournalObject(t, store, 1, 1, validJournalEntryObject(1, 1))
		reader, err := NewJournalReader(store, contracts.JournalPosition{ActiveGeneration: 2, PredecessorSeal: contracts.DeletionWatermark{Generation: 1, SequenceID: 1, EntryHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, SuccessorAllocationID: "allocation-2"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := reader.Observe(context.Background()); !errors.Is(err, contracts.ErrDeletionJournalIncomplete) {
			t.Fatalf("gap error = %v", err)
		}
	})
	t.Run("writer reused across generations", func(t *testing.T) {
		store, err := NewFileObjectStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		first := NewJournal(store, "reused", 1, nil)
		if _, err := first.Seal(context.Background(), 1); err != nil {
			t.Fatal(err)
		}
		second := NewJournal(store, "reused", 2, nil)
		entry, err := second.AppendDeletion(context.Background(), contracts.DeletionEntry{SubjectType: contracts.DeletionSubjectBrain, SubjectID: "brain-1234567890", Outcome: contracts.DeletionIntentRequested})
		if err != nil {
			t.Fatal(err)
		}
		sealBody, found, err := store.Get(context.Background(), contracts.JournalKey(1, 1))
		if err != nil || !found {
			t.Fatalf("read seal: %v", err)
		}
		position := contracts.JournalPosition{ActiveGeneration: 2, PredecessorSeal: contracts.DeletionWatermark{Generation: 1, SequenceID: 1, EntryHash: contracts.HashObject(sealBody)}, SuccessorAllocationID: "allocation-2", LastObjectInGeneration: entry.Watermark}
		reader, err := NewJournalReader(store, position)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := reader.Observe(context.Background()); err == nil || !strings.Contains(err.Error(), "reused") {
			t.Fatalf("writer reuse error = %v", err)
		}
	})
}

func TestReservedAppendRejectsStalePositionWithoutPut(t *testing.T) {
	base, err := NewFileObjectStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	legacy := NewJournal(base, "legacy", 1, nil)
	if _, err := legacy.AppendDeletion(context.Background(), contracts.DeletionEntry{SubjectType: contracts.DeletionSubjectBrain, SubjectID: "brain-1234567890", Outcome: contracts.DeletionIntentRequested}); err != nil {
		t.Fatal(err)
	}
	store := &countingJournalStore{JournalObjectStore: base}
	reserved, err := NewJournalAt(store, "writer-1", contracts.JournalPosition{ActiveGeneration: 1, GenesisIssuanceID: "genesis-1"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = reserved.AppendDeletion(context.Background(), contracts.DeletionEntry{SubjectType: contracts.DeletionSubjectBrain, SubjectID: "brain-2234567890", Outcome: contracts.DeletionIntentRequested})
	if !errors.Is(err, contracts.ErrDeletionJournalHistoryMismatch) || store.puts != 0 {
		t.Fatalf("append error=%v puts=%d", err, store.puts)
	}
}

func TestReadOnlyJournalExportsOnlyObserve(t *testing.T) {
	readerType := reflect.TypeOf((*ReadOnlyJournal)(nil))
	if readerType.NumMethod() != 1 || readerType.Method(0).Name != "Observe" {
		t.Fatalf("read-only method set: %v", readerType)
	}
	contractType := reflect.TypeOf((*contracts.DeletionJournalReader)(nil)).Elem()
	if contractType.NumMethod() != 1 || contractType.Method(0).Name != "Observe" {
		t.Fatalf("reader contract method set: %v", contractType)
	}
}

func TestObserveIncludesSealOnlyWriterUse(t *testing.T) {
	store, err := NewFileObjectStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	entry := putJournalObject(t, store, 1, 1, validJournalEntryObject(1, 1))
	seal := putJournalObject(t, store, 1, 2, contracts.JournalObject{Kind: contracts.JournalKindSeal, Generation: 1, SequenceID: 2, PrevHash: entry.EntryHash, WriterID: "seal-only", RecordedAt: time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC)})
	reader, err := NewJournalReader(store, contracts.JournalPosition{ActiveGeneration: 2, PredecessorSeal: seal, SuccessorAllocationID: "allocation-2"})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := reader.Observe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []contracts.JournalWriterUse{{Generation: 1, WriterID: "writer-1"}, {Generation: 1, WriterID: "seal-only"}}
	if !reflect.DeepEqual(observation.WriterUses, want) {
		t.Fatalf("writer uses = %#v, want %#v", observation.WriterUses, want)
	}
}

func TestObserveRejectsOversizedObjectAndInvalidPage(t *testing.T) {
	t.Run("oversized object", func(t *testing.T) {
		store, err := NewFileObjectStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		object := validJournalEntryObject(1, 1)
		object.SubjectID = strings.Repeat("a", maxObservedObjectBytes)
		body, err := object.Encode()
		if err != nil {
			t.Fatal(err)
		}
		if created, err := store.PutIfAbsent(context.Background(), contracts.JournalKey(1, 1), body); err != nil || !created {
			t.Fatalf("put=%v err=%v", created, err)
		}
		reader, err := NewJournalReader(store, contracts.JournalPosition{ActiveGeneration: 1, GenesisIssuanceID: "genesis-1", LastObjectInGeneration: contracts.DeletionWatermark{Generation: 1, SequenceID: 1, EntryHash: contracts.HashObject(body)}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := reader.Observe(context.Background()); err == nil || !strings.Contains(err.Error(), "byte limit") {
			t.Fatalf("oversize error = %v", err)
		}
	})
	t.Run("page larger than requested", func(t *testing.T) {
		store, err := NewFileObjectStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		wrapped := oversizedPageReader{JournalObjectReader: store}
		reader, err := NewJournalReader(wrapped, contracts.JournalPosition{ActiveGeneration: 1, GenesisIssuanceID: "genesis-1"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := reader.Observe(context.Background()); err == nil || !strings.Contains(err.Error(), "invalid") {
			t.Fatalf("page error = %v", err)
		}
	})
	t.Run("empty page with more", func(t *testing.T) {
		store, err := NewFileObjectStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		reader, err := NewJournalReader(emptyMorePageReader{JournalObjectReader: store}, contracts.JournalPosition{ActiveGeneration: 1, GenesisIssuanceID: "genesis-1"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := reader.Observe(context.Background()); err == nil || !strings.Contains(err.Error(), "invalid") {
			t.Fatalf("empty-more error = %v", err)
		}
	})
}

func TestObserveRejectsMalformedNamespaceKeys(t *testing.T) {
	store, err := NewFileObjectStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if created, err := store.PutIfAbsent(context.Background(), "deletion-journal/not-a-generation", []byte("untrusted")); err != nil || !created {
		t.Fatalf("put malformed key: created=%v err=%v", created, err)
	}
	reader, err := NewJournalReader(store, contracts.JournalPosition{ActiveGeneration: 1, GenesisIssuanceID: "genesis-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Observe(context.Background()); !errors.Is(err, contracts.ErrDeletionJournalIncomplete) {
		t.Fatalf("malformed key error = %v", err)
	}
}

func TestObserveHonorsCancellationBeforeIO(t *testing.T) {
	store, err := NewFileObjectStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	counted := &countingReader{JournalObjectReader: store}
	reader, err := NewJournalReader(counted, contracts.JournalPosition{ActiveGeneration: 1, GenesisIssuanceID: "genesis-1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reader.Observe(ctx); !errors.Is(err, context.Canceled) || counted.calls != 0 {
		t.Fatalf("observe err=%v calls=%d", err, counted.calls)
	}
}

func TestObserveRejectsGenerationOverLimitBeforeListing(t *testing.T) {
	store, err := NewFileObjectStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reader, err := NewJournalReader(store, contracts.JournalPosition{ActiveGeneration: maxObservedGenerations + 1, SuccessorAllocationID: "allocation", PredecessorSeal: contracts.DeletionWatermark{Generation: maxObservedGenerations, SequenceID: 1, EntryHash: strings.Repeat("a", 64)}})
	if err == nil {
		t.Fatal("oversized generation accepted")
	}
	_ = reader
}

type oversizedPageReader struct{ contracts.JournalObjectReader }

func (r oversizedPageReader) ListAfter(context.Context, string, string, int) ([]string, bool, error) {
	return make([]string, observationPageSize+1), false, nil
}

type emptyMorePageReader struct{ contracts.JournalObjectReader }

func (r emptyMorePageReader) ListAfter(context.Context, string, string, int) ([]string, bool, error) {
	return nil, true, nil
}

type countingReader struct {
	contracts.JournalObjectReader
	calls int
}

func (r *countingReader) ListAfter(ctx context.Context, prefix, startAfter string, limit int) ([]string, bool, error) {
	r.calls++
	return r.JournalObjectReader.ListAfter(ctx, prefix, startAfter, limit)
}

func (r *countingReader) Get(ctx context.Context, key string) ([]byte, bool, error) {
	r.calls++
	return r.JournalObjectReader.Get(ctx, key)
}

type countingJournalStore struct {
	contracts.JournalObjectStore
	puts int
}

type rewriteJournalGetStore struct {
	contracts.JournalObjectStore
	key       string
	rewritten []byte
}

func (s *rewriteJournalGetStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if key == s.key {
		return append([]byte(nil), s.rewritten...), true, nil
	}
	return s.JournalObjectStore.Get(ctx, key)
}

func (s *countingJournalStore) PutIfAbsent(ctx context.Context, key string, body []byte) (bool, error) {
	s.puts++
	return s.JournalObjectStore.PutIfAbsent(ctx, key, body)
}

type ambiguousJournalStore struct {
	contracts.JournalObjectStore
	body      []byte
	first     bool
	failReads int
}

func (s *ambiguousJournalStore) PutIfAbsent(ctx context.Context, key string, body []byte) (bool, error) {
	if !s.first {
		s.first = true
		created, err := s.JournalObjectStore.PutIfAbsent(ctx, key, body)
		s.body = append([]byte(nil), body...)
		if err != nil || !created {
			return created, err
		}
		s.failReads = 3
		return false, errors.New("lost response")
	}
	if s.failReads > 0 {
		return false, errors.New("ambiguous retry")
	}
	return s.JournalObjectStore.PutIfAbsent(ctx, key, body)
}

func (s *ambiguousJournalStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	if s.failReads > 0 {
		s.failReads--
		return nil, false, errors.New("read unavailable")
	}
	return s.JournalObjectStore.Get(ctx, key)
}

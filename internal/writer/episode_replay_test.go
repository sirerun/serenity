package writer

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/store"
)

func TestEpisodeJournalIntentBeforeCanonicalAndFailedIntentQuarantines(t *testing.T) {
	ctx := context.Background()
	root, _ := gitRepoFixture(t)
	s := store.NewSourceStore(root)
	q := NewQueue(nil)
	defer q.Close()
	original := writerEpisodeFixture(t)
	if _, err := Episode(ctx, q, s, root, "brain", 0, original, nil); err != nil {
		t.Fatal(err)
	}
	in := store.EpisodeInput{Operation: "exclude", ProducerID: original.ProducerID, ConversationID: original.ConversationID, CommandID: "delete"}
	failed := errors.New("ambiguous external intent")
	var captured store.EpisodeRecord
	calls := 0
	_, err := EpisodeJournaledExclude(ctx, q, s, root, "brain", 0, in, nil, func(r *store.EpisodeRecord) error {
		calls++
		p, e := store.LoadEpisodes(ctx, s)
		if e != nil || len(p.Fences) != 0 {
			t.Fatalf("canonical fence preceded journal %v", e)
		}
		captured = *r
		return failed
	})
	if !errors.Is(err, failed) || calls != 1 {
		t.Fatalf("failed intent acknowledged: %v", err)
	}
	p, err := store.LoadEpisodes(ctx, s)
	if err != nil || len(p.Fences) != 1 {
		t.Fatalf("quarantine absent %v", err)
	}
	if err = EpisodesPublished(ctx, root, p); err != nil {
		t.Fatalf("quarantine not durable %v", err)
	}
	if len(p.SortedEvents()) != 0 {
		t.Fatal("failed append still disclosed originals")
	}
	receipt, err := EpisodeJournaledExclude(ctx, q, s, root, "brain", p.Generation, in, nil, func(r *store.EpisodeRecord) error {
		if !reflect.DeepEqual(captured, *r) {
			t.Fatal("retry changed opaque intent")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := EpisodeReplayFence(ctx, q, s, root, captured)
	if err != nil || !reflect.DeepEqual(receipt, replay) {
		t.Fatalf("unstable replay %v", err)
	}
	if _, err = Episode(ctx, q, s, root, "brain", p.Generation, original, nil); !errors.Is(err, store.ErrEpisodeFenced) {
		t.Fatalf("stale queue escaped fence %v", err)
	}
}

func TestEpisodeReplayFenceExcludesRestoredCheckpointAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	root, _ := gitRepoFixture(t)
	s := store.NewSourceStore(root)
	q := NewQueue(nil)
	defer q.Close()
	in := writerEpisodeFixture(t)
	receipt, err := Episode(ctx, q, s, root, "brain", 0, in, nil)
	if err != nil {
		t.Fatal(err)
	}
	cp := in
	cp.Operation, cp.CommandID, cp.ConversationID, cp.Events = "checkpoint", "checkpoint", "sibling-conversation", nil
	cp.Checkpoint = &store.EpisodeCheckpoint{Text: "derived original", Model: "fixture", CreatedAt: in.Events[0].ObservedAt, Sources: receipt.EventRefs}
	checkpoint, err := Episode(ctx, q, s, root, "brain", 0, cp, nil)
	if err != nil {
		t.Fatal(err)
	}
	exclusion := store.EpisodeInput{Operation: "exclude", ProducerID: in.ProducerID, ConversationID: in.ConversationID, CommandID: "delete"}
	fence := store.EpisodeRecord{Schema: store.EpisodeSchema, BrainID: "brain", Kind: "exclude", CommandKey: store.EpisodeCommandKey(exclusion.ProducerID, exclusion.CommandID), ConversationKey: store.EpisodeConversationKey(exclusion.ProducerID, exclusion.ConversationID), RequestDigest: store.EpisodeHash(exclusion), RecordedAt: time.Now().UTC()}
	for i := 0; i < 2; i++ {
		if _, err = EpisodeReplayFence(ctx, q, s, root, fence); err != nil {
			t.Fatal(err)
		}
	}
	p, err := store.LoadEpisodes(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if p.Generation != 1 || p.RecordEligible(p.Records[checkpoint.CommandKey]) || len(p.SortedEvents()) != 0 {
		t.Fatal("restored evidence or derived checkpoint eligible")
	}
	foreign := fence
	foreign.BrainID = "foreign"
	if _, err = EpisodeReplayFence(ctx, q, s, root, foreign); !errors.Is(err, store.ErrEpisodeConflict) {
		t.Fatalf("foreign brain accepted %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = EpisodeReplayFence(canceled, q, s, root, fence); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled replay %v", err)
	}
}

func TestEpisodeJournaledFenceLimitCannotAcknowledgeMissingCanonicalFence(t *testing.T) {
	ctx := context.Background()
	root, git := gitRepoFixture(t)
	s := store.NewSourceStore(root)
	q := NewQueue(nil)
	defer q.Close()
	for i := 0; i < store.EpisodeMaxFences; i++ {
		in := store.EpisodeInput{Operation: "exclude", ProducerID: "p", ConversationID: fmt.Sprintf("c-%d", i), CommandID: fmt.Sprintf("d-%d", i)}
		_, err := s.PutEpisode(store.EpisodeRecord{Schema: store.EpisodeSchema, BrainID: "brain", Kind: "exclude", CommandKey: store.EpisodeCommandKey(in.ProducerID, in.CommandID), ConversationKey: store.EpisodeConversationKey(in.ProducerID, in.ConversationID), RequestDigest: store.EpisodeHash(in), Generation: uint64(i + 1), RecordedAt: time.Now().UTC()})
		if err != nil {
			t.Fatal(err)
		}
	}
	git("add", "brain/sources")
	git("commit", "-m", "full reserved fence fixture")
	called := false
	in := store.EpisodeInput{Operation: "exclude", ProducerID: "p", ConversationID: "overflow", CommandID: "overflow"}
	_, err := EpisodeJournaledExclude(ctx, q, s, root, "brain", store.EpisodeMaxFences, in, nil, func(*store.EpisodeRecord) error { called = true; return nil })
	if !called || !errors.Is(err, store.ErrEpisodeLimit) {
		t.Fatalf("limit lost intent or acknowledged: called=%v err=%v", called, err)
	}
}

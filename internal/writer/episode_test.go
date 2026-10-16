package writer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/store"
)

func writerEpisodeFixture(t *testing.T) store.EpisodeInput {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/conformance/episodes/append.json")
	if err != nil {
		t.Fatal(err)
	}
	var req struct {
		Mutation store.EpisodeInput `json:"mutation"`
	}
	if err = json.Unmarshal(raw, &req); err != nil {
		t.Fatal(err)
	}
	return req.Mutation
}
func TestEpisodePublicationRetryDedupAndSourceDate(t *testing.T) {
	ctx := context.Background()
	root, git := gitRepoFixture(t)
	s := store.NewSourceStore(root)
	q := NewQueue(nil)
	defer q.Close()
	in := writerEpisodeFixture(t)
	first, err := Episode(ctx, q, s, root, "synthetic-brain", 0, in, nil)
	if err != nil {
		t.Fatal(err)
	}
	originalHead := git("rev-parse", "HEAD")
	raw := git("show", "HEAD:brain/sources/"+first.SourceSHA256[:2]+"/"+first.SourceSHA256+"/bytes")
	if !strings.Contains(raw, "Exact visible text.") {
		t.Fatal("acknowledged before publication")
	}
	second, err := Episode(ctx, q, s, root, "synthetic-brain", 0, in, nil)
	if err != nil || !reflect.DeepEqual(first, second) || git("rev-parse", "HEAD") != originalHead {
		t.Fatalf("unstable retry %+v %v", second, err)
	}
	in.Events[0].Text = "changed"
	if _, err = Episode(ctx, q, s, root, "synthetic-brain", 0, in, nil); !errors.Is(err, store.ErrEpisodeConflict) {
		t.Fatalf("changed same command %v", err)
	}
	in = writerEpisodeFixture(t)
	in.CommandID = "import-retry-new-batch"
	duplicate, err := Episode(ctx, q, s, root, "synthetic-brain", 0, in, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, _, err := s.Read(duplicate.SourceSHA256)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("Exact visible text.")) {
		t.Fatal("reimport duplicated plaintext")
	}
	p, err := store.LoadEpisodes(ctx, s)
	if err != nil || len(p.Events) != 1 || len(p.Records) != 2 {
		t.Fatalf("projection %v %+v", err, p)
	}
	in.CommandID = "changed-event-new-command"
	in.Events[0].Text = "changed same event"
	if _, err = Episode(ctx, q, s, root, "synthetic-brain", 0, in, nil); !errors.Is(err, store.ErrEpisodeConflict) {
		t.Fatalf("changed event %v", err)
	}
	in = writerEpisodeFixture(t)
	in.CommandID = "sequence-collision"
	in.Events[0].ID = "new-event-same-sequence"
	if _, err = Episode(ctx, q, s, root, "synthetic-brain", 0, in, nil); !errors.Is(err, store.ErrEpisodeConflict) {
		t.Fatalf("sequence collision %v", err)
	}
}
func TestEpisodeExclusionFencesCheckpointAndReplay(t *testing.T) {
	ctx := context.Background()
	root, _ := gitRepoFixture(t)
	s := store.NewSourceStore(root)
	q := NewQueue(nil)
	defer q.Close()
	in := writerEpisodeFixture(t)
	receipt, err := Episode(ctx, q, s, root, "synthetic-brain", 0, in, nil)
	if err != nil {
		t.Fatal(err)
	}
	cp := in
	cp.Operation = "checkpoint"
	cp.CommandID = "checkpoint-1"
	cp.ConversationID = "receiving-conversation"
	cp.Events = nil
	cp.Checkpoint = &store.EpisodeCheckpoint{Text: "Exact pending work; verify current permissions.", Model: "client-provided/no-server-model", CreatedAt: in.Events[0].ObservedAt, Sources: receipt.EventRefs}
	checkpoint, err := Episode(ctx, q, s, root, "synthetic-brain", 0, cp, nil)
	if err != nil {
		t.Fatal(err)
	}
	exclusion := store.EpisodeInput{Operation: "exclude", ProducerID: in.ProducerID, ConversationID: in.ConversationID, CommandID: "exclude-1"}
	fence, err := Episode(ctx, q, s, root, "synthetic-brain", 0, exclusion, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := s.Read(fence.SourceSHA256)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{in.ConversationID, in.ProducerID, in.Events[0].Text, exclusion.CommandID} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatal("fence retained original identity/content")
		}
	}
	p, err := store.LoadEpisodes(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if p.Generation != 1 || len(p.SortedEvents()) != 0 || p.RecordEligible(p.Records[checkpoint.CommandKey]) {
		t.Fatal("exclusion failed")
	}
	if _, err = Episode(ctx, q, s, root, "synthetic-brain", 0, in, nil); !errors.Is(err, store.ErrEpisodeGeneration) {
		t.Fatalf("stale generation %v", err)
	}
	if _, err = Episode(ctx, q, s, root, "synthetic-brain", 1, in, nil); !errors.Is(err, store.ErrEpisodeFenced) {
		t.Fatalf("replay resurrected %v", err)
	}
	if _, err = Episode(ctx, q, s, root, "synthetic-brain", 1, cp, nil); !errors.Is(err, store.ErrEpisodeFenced) {
		t.Fatalf("checkpoint resurrected %v", err)
	}
	replay, err := Episode(ctx, q, s, root, "synthetic-brain", 1, exclusion, nil)
	if err != nil || !reflect.DeepEqual(fence, replay) {
		t.Fatalf("fence replay %v", err)
	}
	if _, _, err = s.Read(receipt.SourceSHA256); err != nil {
		t.Fatal("exclusion unexpectedly claimed physical deletion")
	}
}
func TestEpisodeGenerationRecheckedInsideQueue(t *testing.T) {
	ctx := context.Background()
	root, _ := gitRepoFixture(t)
	s := store.NewSourceStore(root)
	q := NewQueue(nil)
	defer q.Close()
	in := writerEpisodeFixture(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	exclusion := store.EpisodeInput{Operation: "exclude", ProducerID: in.ProducerID, ConversationID: in.ConversationID, CommandID: "exclude-before-append"}
	go func() {
		_, err := Episode(ctx, q, s, root, "synthetic-brain", 0, exclusion, func(context.Context) error { close(entered); <-release; return nil })
		done <- err
	}()
	<-entered
	var wg sync.WaitGroup
	wg.Add(1)
	var appendErr error
	go func() { defer wg.Done(); _, appendErr = Episode(ctx, q, s, root, "synthetic-brain", 0, in, nil) }()
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if !errors.Is(appendErr, store.ErrEpisodeGeneration) {
		t.Fatalf("queued stale append %v", appendErr)
	}
	p, err := store.LoadEpisodes(ctx, s)
	if err != nil || len(p.Events) != 0 {
		t.Fatalf("published stale event %v", err)
	}
}
func TestEpisodeFailurePublicationAndAuthority(t *testing.T) {
	ctx := context.Background()
	root, git := gitRepoFixture(t)
	s := store.NewSourceStore(root)
	q := NewQueue(nil)
	defer q.Close()
	in := writerEpisodeFixture(t)
	denied := errors.New("revoked before publication")
	if _, err := Episode(ctx, q, s, root, "synthetic-brain", 0, in, func(context.Context) error { return denied }); !errors.Is(err, denied) {
		t.Fatalf("authority %v", err)
	}
	p, err := store.LoadEpisodes(ctx, s)
	if err != nil || len(p.Records) != 0 {
		t.Fatalf("authority wrote bytes %v", err)
	}
	// A failing real pre-commit hook leaves a retryable source but no receipt.
	hook := filepath.Join(root, ".git", "hooks", "pre-commit")
	if err = os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	head := git("rev-parse", "HEAD")
	receipt, err := Episode(ctx, q, s, root, "synthetic-brain", 0, in, nil)
	if err == nil || receipt.SourceSHA256 != "" || git("rev-parse", "HEAD") != head {
		t.Fatal("failed publication acknowledged")
	}
	p, err = store.LoadEpisodes(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if err = EpisodesPublished(ctx, root, p); err == nil {
		t.Fatal("unpublished projection considered durable")
	}
	if err = os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	receipt, err = Episode(ctx, q, s, root, "synthetic-brain", 0, in, nil)
	if err != nil || receipt.SourceSHA256 == "" {
		t.Fatalf("retry failed %v", err)
	}
	p, err = store.LoadEpisodes(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if err = EpisodesPublished(ctx, root, p); err != nil {
		t.Fatal(err)
	}
	meta := filepath.Join(s.DirFor(receipt.SourceSHA256), "meta.yaml")
	raw, err := os.ReadFile(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(meta, append(raw, []byte("# unpublished metadata\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if err = EpisodesPublished(ctx, root, p); err == nil {
		t.Fatal("dirty metadata accepted")
	}
}
func TestEpisodeArchiveQuotaReservesFenceCapacity(t *testing.T) {
	ctx := context.Background()
	root, git := gitRepoFixture(t)
	s := store.NewSourceStore(root)
	q := NewQueue(nil)
	defer q.Close()
	in := writerEpisodeFixture(t)
	// Seed a full canonical archive with one original and 255 content-free
	// deduplicated batches, then exercise real writer admission at the boundary.
	for i := 0; i < store.EpisodeMaxRecords; i++ {
		candidate := in
		candidate.CommandID = fmt.Sprintf("seed-%d", i)
		r := store.EpisodeRecord{Schema: store.EpisodeSchema, BrainID: "synthetic-brain", Kind: "append", CommandKey: store.EpisodeCommandKey(candidate.ProducerID, candidate.CommandID), ConversationKey: store.EpisodeConversationKey(candidate.ProducerID, candidate.ConversationID), RequestDigest: store.EpisodeHash(candidate), RecordedAt: time.Now().UTC(), Refs: []store.EpisodeRef{{EventKey: store.EpisodeEventKey(candidate.ProducerID, candidate.ConversationID, candidate.Events[0].ID), Digest: store.EpisodeHash(candidate.Events[0])}}}
		if i > 0 {
			candidate.Events = nil
		}
		r.Input = &candidate
		if _, err := s.PutEpisode(r); err != nil {
			t.Fatal(err)
		}
	}
	git("add", "brain/sources")
	git("commit", "-m", "full synthetic archive")
	if _, err := Episode(ctx, q, s, root, "synthetic-brain", 0, in, nil); !errors.Is(err, store.ErrEpisodeLimit) {
		t.Fatalf("full archive admitted: %v", err)
	}
	exclusion := store.EpisodeInput{Operation: "exclude", ProducerID: in.ProducerID, ConversationID: in.ConversationID, CommandID: "fence-at-capacity"}
	if _, err := Episode(ctx, q, s, root, "synthetic-brain", 0, exclusion, nil); err != nil {
		t.Fatal(err)
	}
	p, err := store.LoadEpisodes(ctx, s)
	if err != nil || p.Generation != 1 || len(p.SortedEvents()) != 0 {
		t.Fatalf("fence at capacity: %v", err)
	}
}

func TestEpisodeQueueOwnsDetachedOriginals(t *testing.T) {
	ctx := context.Background()
	root, _ := gitRepoFixture(t)
	s := store.NewSourceStore(root)
	q := NewQueue(nil)
	defer q.Close()
	in := writerEpisodeFixture(t)
	originalText := in.Events[0].Text
	originalAttachment := append([]byte(nil), in.Events[0].Attachments[0].Data...)
	entered, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, err := Episode(ctx, q, s, root, "synthetic-brain", 0, in, func(context.Context) error { close(entered); <-release; return nil })
		done <- err
	}()
	<-entered
	in.Events[0].Text = "caller changed after admission"
	in.Events[0].Attachments[0].Data[0] = 42
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	p, err := store.LoadEpisodes(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	events := p.SortedEvents()
	if len(events) != 1 || events[0].Event.Text != originalText || !bytes.Equal(events[0].Event.Attachments[0].Data, originalAttachment) {
		t.Fatal("queued original retained caller-owned slices")
	}
}

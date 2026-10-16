package store

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestEpisodeReplayDiscoverySharesProjectionValidation(t *testing.T) {
	s := NewSourceStore(t.TempDir())
	r := episodeFixtureRecord(t)
	if _, err := s.PutEpisode(r); err != nil {
		t.Fatal(err)
	}
	in := EpisodeInput{Operation: "exclude", ProducerID: r.Input.ProducerID, ConversationID: r.Input.ConversationID, CommandID: "exclude"}
	fence := EpisodeRecord{Schema: EpisodeSchema, BrainID: r.BrainID, Kind: "exclude", CommandKey: EpisodeCommandKey(in.ProducerID, in.CommandID), ConversationKey: EpisodeConversationKey(in.ProducerID, in.ConversationID), RequestDigest: EpisodeHash(in), Generation: 1, RecordedAt: r.RecordedAt}
	if _, err := s.PutEpisode(fence); err != nil {
		t.Fatal(err)
	}
	interactive, err := LoadEpisodes(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := LoadEpisodesForReplay(context.Background(), s)
	if err != nil || !reflect.DeepEqual(interactive, replay) {
		t.Fatalf("projection differs: %v", err)
	}
	if len(replay.SortedEvents()) != 0 {
		t.Fatal("replay projection lost exclusion")
	}
	// Both loaders reject conflicts even though streaming discovery has no
	// lexicographic ordering requirement.
	conflicting := fence
	conflicting.RequestDigest = EpisodeHash("conflicting digest")
	if _, err = s.PutEpisode(conflicting); err != nil {
		t.Fatal(err)
	}
	for _, load := range []func(context.Context, *SourceStore) (*EpisodeProjection, error){LoadEpisodes, LoadEpisodesForReplay} {
		if _, err = load(context.Background(), s); !errors.Is(err, ErrEpisodeConflict) {
			t.Fatalf("conflict accepted: %v", err)
		}
	}
}

func TestEpisodeReplayDiscoveryRetainsEpisodeBounds(t *testing.T) {
	for _, mode := range []string{"fences", "records", "record-bytes", "metadata-bytes"} {
		t.Run(mode, func(t *testing.T) {
			s := NewSourceStore(t.TempDir())
			if mode == "record-bytes" || mode == "metadata-bytes" {
				src, err := s.PutEpisode(episodeFixtureRecord(t))
				if err != nil {
					t.Fatal(err)
				}
				path, size := s.bytesPath(src.SHA256), int64(EpisodeMaxRecordBytes+1)
				if mode == "metadata-bytes" {
					path, size = s.metaPath(src.SHA256), 64<<10+1
				}
				if err = os.Truncate(path, size); err != nil {
					t.Fatal(err)
				}
			} else {
				count := EpisodeMaxRecords + 1
				if mode == "fences" {
					count = EpisodeMaxFences + 1
				}
				for i := 0; i < count; i++ {
					r := episodeFixtureRecord(t)
					if mode == "fences" {
						in := EpisodeInput{Operation: "exclude", ProducerID: "p", ConversationID: fmt.Sprintf("c-%d", i), CommandID: fmt.Sprintf("d-%d", i)}
						r = EpisodeRecord{Schema: EpisodeSchema, BrainID: "synthetic-brain", Kind: "exclude", CommandKey: EpisodeCommandKey(in.ProducerID, in.CommandID), ConversationKey: EpisodeConversationKey(in.ProducerID, in.ConversationID), RequestDigest: EpisodeHash(in), Generation: uint64(i + 1), RecordedAt: time.Now().UTC()}
					} else {
						r.Input.CommandID = fmt.Sprintf("command-%d", i)
						r.Input.ConversationID = fmt.Sprintf("conversation-%d", i)
						r.CommandKey = EpisodeCommandKey(r.Input.ProducerID, r.Input.CommandID)
						r.ConversationKey = EpisodeConversationKey(r.Input.ProducerID, r.Input.ConversationID)
						r.RequestDigest = EpisodeHash(*r.Input)
						r.Refs = []EpisodeRef{{EpisodeEventKey(r.Input.ProducerID, r.Input.ConversationID, r.Input.Events[0].ID), EpisodeHash(r.Input.Events[0])}}
					}
					if _, err := s.PutEpisode(r); err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err := LoadEpisodesForReplay(context.Background(), s); !errors.Is(err, ErrEpisodeLimit) {
				t.Fatalf("episode %s bound relaxed: %v", mode, err)
			}
		})
	}
}

func TestEpisodeReplayStreamingTraversalCancellationAndStages(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 300; i++ {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("entry-%03d", i)), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	seen := 0
	if err := walkEpisodeReplaySources(context.Background(), root, func(_ string, _ fs.DirEntry, err error) error { seen++; return err }); err != nil || seen != 301 {
		t.Fatalf("paged traversal missed entries: %d %v", seen, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	seen = 0
	err := walkEpisodeReplaySources(ctx, root, func(_ string, _ fs.DirEntry, err error) error {
		seen++
		if seen == 150 {
			cancel()
		}
		return err
	})
	if !errors.Is(err, context.Canceled) || seen != 150 {
		t.Fatalf("traversal continued after cancellation: %d %v", seen, err)
	}
	if _, err = LoadEpisodesForReplay(ctx, NewSourceStore(t.TempDir())); !errors.Is(err, context.Canceled) {
		t.Fatalf("empty discovery ignored cancellation: %v", err)
	}
	s := NewSourceStore(t.TempDir())
	stage := filepath.Join(s.Root, "brain", "sources", "aa", ".source-stage-unpublished")
	if err = os.MkdirAll(stage, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(stage, "meta.yaml"), []byte("malformed unpublished metadata"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadEpisodesForReplay(context.Background(), s); err != nil {
		t.Fatalf("staging skip changed: %v", err)
	}
	if err = os.Symlink(root, filepath.Join(s.Root, "brain", "sources", "redirect")); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadEpisodesForReplay(context.Background(), s); err == nil {
		t.Fatal("source symlink followed")
	}
}

func TestEpisodeReplayTraversalBoundsDirectoryDepth(t *testing.T) {
	s := NewSourceStore(t.TempDir())
	path := filepath.Join(s.Root, "brain", "sources")
	for i := 0; i < 66; i++ {
		path = filepath.Join(path, "nested")
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadEpisodesForReplay(context.Background(), s); !errors.Is(err, ErrEpisodeInvalid) {
		t.Fatalf("unbounded directory nesting: %v", err)
	}
}

package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/domain"
)

func episodeFixture(t *testing.T) EpisodeInput {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/conformance/episodes/append.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Mutation EpisodeInput `json:"mutation"`
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture.Mutation
}
func episodeFixtureRecord(t *testing.T) EpisodeRecord {
	t.Helper()
	in := episodeFixture(t)
	return EpisodeRecord{Schema: EpisodeSchema, BrainID: "synthetic-brain", Kind: "append", CommandKey: EpisodeCommandKey(in.ProducerID, in.CommandID), ConversationKey: EpisodeConversationKey(in.ProducerID, in.ConversationID), RequestDigest: EpisodeHash(in), RecordedAt: time.Date(2026, 10, 16, 10, 0, 0, 0, time.UTC), Input: &in, Refs: []EpisodeRef{{EpisodeEventKey(in.ProducerID, in.ConversationID, in.Events[0].ID), EpisodeHash(in.Events[0])}}}
}
func TestEpisodeSourceExactOriginalAndReservedBoundary(t *testing.T) {
	s := NewSourceStore(t.TempDir())
	r := episodeFixtureRecord(t)
	src, err := s.PutEpisode(r)
	if err != nil {
		t.Fatal(err)
	}
	raw, meta, err := s.Read(src.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if !meta.OccurredAt.Equal(r.Input.Events[0].OccurredAt) || meta.IndexOnly || meta.Kind != SourceKindEpisode {
		t.Fatalf("metadata %+v", meta)
	}
	var actual EpisodeRecord
	if err = json.Unmarshal(raw, &actual); err != nil {
		t.Fatal(err)
	}
	if actual.Input.Events[0].Text != r.Input.Events[0].Text || !bytes.Equal(actual.Input.Events[0].Attachments[0].Data, r.Input.Events[0].Attachments[0].Data) {
		t.Fatal("original changed")
	}
	if strings.Count(string(raw), "Exact visible text.") != 1 || bytes.Contains(raw, []byte(`"request":`)) {
		t.Fatal("duplicate request envelope")
	}
	p, err := LoadEpisodes(context.Background(), s)
	if err != nil || len(p.Events) != 1 {
		t.Fatalf("projection: %+v %v", p, err)
	}
	for _, kind := range []string{SourceKindEpisode, "MEMORY_EPISODE"} {
		if _, err = s.Write(raw, domain.Source{Kind: kind}); err == nil {
			t.Fatal("ordinary ingestion accepted reserved archive")
		}
	}
	memory, err := LoadMemoryProjection(s)
	if err != nil {
		t.Fatal(err)
	}
	if len(memory.All()) != 0 || MemoryEligible(memory, src.SHA256, false, time.Now()) || MemoryEligible(memory, src.SHA256, true, time.Now()) {
		t.Fatal("archive escaped into ordinary memory")
	}
	if _, err = s.TombstoneAt(src.SHA256, nil, time.Now()); err == nil {
		t.Fatal("legacy tombstone erased archive")
	}
}
func TestEpisodeValidationLimitsAndAttribution(t *testing.T) {
	cases := map[string]func(*EpisodeInput){
		"event-count":         func(in *EpisodeInput) { in.Events = make([]EpisodeEvent, 65) },
		"text-size":           func(in *EpisodeInput) { in.Events[0].Text = strings.Repeat("x", EpisodeMaxTextBytes+1) },
		"attachment-digest":   func(in *EpisodeInput) { in.Events[0].Attachments[0].SHA256 = strings.Repeat("0", 64) },
		"attachment-size":     func(in *EpisodeInput) { in.Events[0].Attachments[0].Data = make([]byte, EpisodeMaxAttachmentBytes+1) },
		"duplicate-event":     func(in *EpisodeInput) { in.Events = append(in.Events, in.Events[0]) },
		"negative-sequence":   func(in *EpisodeInput) { in.Events[0].Sequence = -1 },
		"missing-source-date": func(in *EpisodeInput) { in.Events[0].OccurredAt = time.Time{} },
		"path-id":             func(in *EpisodeInput) { in.ConversationID = "../private" },
		"invalid-utf8":        func(in *EpisodeInput) { in.Events[0].Text = string([]byte{255}) },
		"hidden-reasoning":    func(in *EpisodeInput) { in.Events[0].Type = "hidden_reasoning" },
		"empty-batch":         func(in *EpisodeInput) { in.Events = nil },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			in := episodeFixture(t)
			change(&in)
			if ValidateEpisodeInput(in) == nil {
				t.Fatal("accepted invalid input")
			}
		})
	}
}
func TestEpisodeProjectionRejectsCorruptionAndSymlinks(t *testing.T) {
	for _, mode := range []string{"digest", "index-only", "missing-bytes", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			s := NewSourceStore(t.TempDir())
			r := episodeFixtureRecord(t)
			if mode == "digest" {
				r.RequestDigest = strings.Repeat("a", 64)
			}
			src, err := s.PutEpisode(r)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "index-only":
				p := filepath.Join(s.DirFor(src.SHA256), "meta.yaml")
				b, _ := os.ReadFile(p)
				if err = os.WriteFile(p, append(b, []byte("index_only: true\n")...), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing-bytes":
				if err = os.Remove(filepath.Join(s.DirFor(src.SHA256), "bytes")); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err = os.Symlink(t.TempDir(), filepath.Join(s.Root, "brain", "sources", "link")); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = LoadEpisodes(context.Background(), s); err == nil {
				t.Fatal("corrupt archive accepted")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := NewSourceStore(t.TempDir())
	if err := os.MkdirAll(filepath.Join(s.Root, "brain", "sources"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadEpisodes(ctx, s); !errors.Is(err, context.Canceled) {
		t.Fatalf("context %v", err)
	}
}

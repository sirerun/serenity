package index

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/store"
)

func TestEpisodeNeverIndexesOrAuthorizesStaleGenericHits(t *testing.T) {
	root, eng := sourcePolicyIndex(t)
	s := store.NewSourceStore(root)
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
	in := req.Mutation
	rec := store.EpisodeRecord{Schema: store.EpisodeSchema, BrainID: "synthetic-brain", Kind: "append", CommandKey: store.EpisodeCommandKey(in.ProducerID, in.CommandID), ConversationKey: store.EpisodeConversationKey(in.ProducerID, in.ConversationID), RequestDigest: store.EpisodeHash(in), RecordedAt: time.Now().UTC(), Input: &in, Refs: []store.EpisodeRef{{EventKey: store.EpisodeEventKey(in.ProducerID, in.ConversationID, in.Events[0].ID), Digest: store.EpisodeHash(in.Events[0])}}}
	src, err := s.PutEpisode(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err = Rebuild(context.Background(), root, config.Default(), eng); err != nil {
		t.Fatal(err)
	}
	hits, err := eng.AllChunks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, hit := range hits {
		if hit.SourceSHA256 == src.SHA256 {
			t.Fatal("archive indexed as ordinary source")
		}
	}
	for _, kind := range []string{store.SourceKindEpisode, "file", store.SourceKindMemoryFact} {
		hit := Hit{ChunkRef: "src:" + src.SHA256 + ":0-20", Text: in.Events[0].Text, SourceSHA256: src.SHA256, Kind: kind}
		if importedPolicy(t, root, true, false)(hit) || importedPolicy(t, root, false, true)(hit) || importedPolicy(t, root, false, false)(hit) {
			t.Fatal("archive stale hit disclosed")
		}
	}
}

package dashboard

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/store"
)

func TestEpisodeExcludedFromLegacyInspectorSourceCatalog(t *testing.T) {
	root := t.TempDir()
	s := store.NewSourceStore(root)
	raw, err := os.ReadFile("../../../testdata/conformance/episodes/append.json")
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
	if _, err = s.PutEpisode(rec); err != nil {
		t.Fatal(err)
	}
	sources, facts, expiries, err := readInspectorSourceCatalog(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 0 || len(facts) != 0 || len(expiries) != 0 {
		t.Fatal("legacy inspector exposed archive")
	}
}

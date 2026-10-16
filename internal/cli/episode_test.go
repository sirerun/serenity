package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/store"
)

func TestEpisodeExcludedBeforeOrdinaryExtraction(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, "archive must not reach provider", 500)
	}))
	defer server.Close()
	t.Setenv("OPENAI_API_KEY", "synthetic-key")
	t.Setenv("OPENAI_BASE_URL", server.URL)
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	root := initBrainRepo(t)
	configureGitIdentity(t, root)
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
	if _, err = s.PutEpisode(rec); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Models.Provider = ""
	cfg.Models.Extraction = "test-extract@v1"
	var out bytes.Buffer
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()
	if err = extractClaims(context.Background(), root, cfg, nil, disposition.NewStore(eng), &out); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("episode text escaped into ordinary extraction")
	}
	if !bytes.Contains(out.Bytes(), []byte("0")) {
		t.Fatalf("expected extraction summary, got %s", out.String())
	}
}

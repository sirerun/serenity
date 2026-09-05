package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/providers"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/index"
)

// fakeExtractionServer stands a real net/http server in for an
// OpenAI-compatible chat-completions API (no real API key, no real network
// egress -- see providers.BuildExtractionRouter's local-server path). Every request
// gets back one fixed, vocabulary-valid extraction candidate: a pure
// function of nothing but the fixture text this test seeds, so repeated
// calls over an unchanged source always produce the exact same candidate,
// matching T1.8's own documented determinism assumption for a real
// extraction model at a fixed pin.
func fakeExtractionServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices": [{"message": {"role": "assistant", "content": "{\"observations\":[{\"subject\":\"acme\",\"predicate\":\"works_at\",\"object\":\"Acme Corp\",\"confidence\":0.9}]}"}}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 5}
		}`))
	}))
	t.Cleanup(server.Close)
	return server
}

// fakeEmbeddingsServer stands a real net/http server in for an
// OpenAI-compatible /embeddings API. The returned vector is a pure,
// deterministic function of the request's own "input" text -- the same
// assumption internal/index/vectors_test.go's
// TestVectorsParticipateInRebuildIdentity documents for a real embedding
// endpoint at a fixed model version -- so a wiped-and-rebuilt index
// re-embeds to the exact same bytes.
func fakeEmbeddingsServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var v [3]float32
		for i, ch := range req.Input {
			v[i%3] += float32(ch)
		}
		vec, err := json.Marshal(v[:])
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("content-type", "application/json")
		_, _ = fmt.Fprintf(w, `{"data":[{"embedding":%s}],"usage":{"prompt_tokens":1,"total_tokens":1}}`, vec)
	}))
	t.Cleanup(server.Close)
	return server
}

// TestSyncExtractEndToEnd is T1.15's acc line, run over a real net/http
// pipeline (fake local servers standing in for the model providers, never
// a fake router.Completer/Embedder -- see the two fake*Server helpers):
// `sync && extract all` twice on the fixture brain adds zero sources and
// zero claims the second time; wiping .serenity/ then syncing reproduces
// the dump byte-identically, including vectors, under the pinned set.
func TestSyncExtractEndToEnd(t *testing.T) {
	requireGit(t)
	ctx := context.Background()

	extServer := fakeExtractionServer(t)
	embServer := fakeEmbeddingsServer(t)

	t.Setenv("OPENAI_BASE_URL", extServer.URL)
	t.Setenv("OPENAI_EMBEDDINGS_BASE_URL", embServer.URL)
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "")

	root := t.TempDir()
	var initOut bytes.Buffer
	if err := runInit(root, &initOut); err != nil {
		t.Fatal(err)
	}
	// A local git identity, not the ambient global config, so this test
	// (the first to reach a real `git commit` via runSync/runExtract's
	// writer.Flush calls) passes on CI runners with no global identity --
	// same convention as internal/writer/commit_test.go and
	// internal/cli/doctor_test.go's pushFixture.
	configureGitIdentity(t, root)

	// Fixture material for the file connector. Backdated well past the 2s
	// debounce (file.DefaultDebounce) so the very first poll -- which runs
	// milliseconds after the file is written -- already treats it as
	// stable, instead of the test racing a real sleep.
	dropDir := t.TempDir()
	notePath := filepath.Join(dropDir, "note.txt")
	if err := os.WriteFile(notePath, []byte("Alice works at Acme Corp."), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-10 * time.Second)
	if err := os.Chtimes(notePath, old, old); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(root, config.FileName)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	// ADR 013: config.Default() seeds Models.Provider: "openrouter" for a
	// new brain (runInit above), which would otherwise route this
	// fixture's extraction call through the real OpenRouter API instead
	// of the local fakeExtractionServer this test stands up -- clear it
	// so buildChatProvider's substring-inference fallback (this test's
	// pre-ADR-013 behavior) governs, exactly as OPENAI_BASE_URL above
	// expects.
	cfg.Models.Provider = ""
	cfg.Models.Extraction = "test-extract@v1"
	cfg.Models.Embedding = "test-embed@v1"
	cfg.Connectors = map[string]any{
		"file": map[string]any{"path": dropDir},
	}
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatal(err)
	}

	runOnce := func() {
		t.Helper()
		var syncOut, extractOut bytes.Buffer
		if err := runSync(ctx, root, &syncOut); err != nil {
			t.Fatalf("sync: %v\noutput:\n%s", err, syncOut.String())
		}
		if err := runExtract(ctx, root, &extractOut); err != nil {
			t.Fatalf("extract: %v\noutput:\n%s", err, extractOut.String())
		}
	}
	readStats := func() (map[string]int64, string) {
		t.Helper()
		eng, err := providers.OpenIndex(root)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = eng.Close() }()
		stats, err := eng.Stats(ctx)
		if err != nil {
			t.Fatal(err)
		}
		dump, err := index.DumpString(ctx, eng)
		if err != nil {
			t.Fatal(err)
		}
		return stats, dump
	}

	runOnce()
	stats1, dump1 := readStats()

	if stats1["claims"] == 0 {
		t.Fatalf("expected at least one claim written on the first run, stats: %+v\ndump:\n%s", stats1, dump1)
	}
	if stats1["entities"] == 0 {
		t.Fatalf("expected at least one entity on the first run, stats: %+v", stats1)
	}
	if stats1["vectors"] == 0 {
		t.Fatalf("expected at least one embedded vector on the first run, stats: %+v", stats1)
	}

	// Run "sync && extract all" a second time over unchanged input: the
	// file connector's own cursor already sees nothing new, and even if it
	// did, source-store content-address dedup plus ingest's per-target
	// claim-id dedup make it a no-op either way -- zero new sources, zero
	// new claims.
	runOnce()
	stats2, _ := readStats()
	if stats2["entities"] != stats1["entities"] {
		t.Fatalf("entities changed on second identical run: %d -> %d", stats1["entities"], stats2["entities"])
	}
	if stats2["claims"] != stats1["claims"] {
		t.Fatalf("claims changed on second identical run: %d -> %d", stats1["claims"], stats2["claims"])
	}

	// Wipe .serenity/ entirely (the derived index, including job/cursor
	// history) and rerun: canonical brain/ content is untouched, so the
	// rebuilt dump -- including re-embedded vectors -- must reproduce byte
	// for byte.
	if err := os.RemoveAll(filepath.Join(root, ".serenity")); err != nil {
		t.Fatal(err)
	}
	runOnce()
	_, dump3 := readStats()

	if dump1 != dump3 {
		t.Fatalf("rebuild after wiping .serenity/ not byte-identical\n--- before ---\n%s\n--- after ---\n%s", dump1, dump3)
	}
	if !strings.Contains(dump1, "vectors\t") {
		t.Fatalf("dump has no vectors rows -- test did not exercise the vectors table:\n%s", dump1)
	}
}

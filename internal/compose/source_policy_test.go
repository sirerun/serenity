package compose

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/router"
	"github.com/sirerun/serenity/internal/store"
)

type composeCountingEmbedder struct{ calls int }

func (*composeCountingEmbedder) ModelVersion() string { return "count@v1" }
func (e *composeCountingEmbedder) Embed(context.Context, string) ([]float32, error) {
	e.calls++
	return []float32{1, 0}, nil
}

func TestComposeSourcePolicyAndSingleQueryEmbedding(t *testing.T) {
	root := reviewComposeRoot(t)
	now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	ss := store.NewSourceStore(root)
	src, err := ss.WriteMemoryFact(store.MemoryFactPayload{FormatVersion: 1, RecordType: store.SourceKindMemoryFact, LegacyID: 1, Fact: "sharedreviewquery attributed report", Provenance: "verbatim review attribution", Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityWorld, CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	ranked := reviewRankedSources{hits: []index.Hit{{ChunkRef: "fact:" + src.SHA256, Text: "sharedreviewquery attributed report", SourceSHA256: src.SHA256, Kind: store.SourceKindMemoryFact}}}
	var prompt string
	provider := &fakeProvider{modelVersion: "review@v1", resp: router.Response{Text: "Report [source:" + src.SHA256 + "]."}, sentPrompt: &prompt}
	embedding := &composeCountingEmbedder{}
	c := New(root, config.Default(), ranked, embedding, newTestRouter(provider), "review@v1")
	clockCalls := 0
	c.now = func() time.Time { clockCalls++; return now }
	result, err := c.AskWithOptions(context.Background(), "sharedreviewquery", AskOptions{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if clockCalls != 0 {
		t.Fatalf("injected query clock ignored: read composer clock %d times", clockCalls)
	}
	if embedding.calls != 1 {
		t.Fatalf("one Ask embeds same query %d times", embedding.calls)
	}
	if len(result.SourceCitations) != 1 || !strings.Contains(prompt, "verbatim review attribution") {
		t.Fatalf("eligible raw source absent from actual provider or citation: %+v %q", result, prompt)
	}
	// World visibility does not grant permission to send index-only bytes abroad.
	meta := filepath.Join(ss.DirFor(src.SHA256), "meta.yaml")
	f, err := os.OpenFile(meta, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("index_only: true\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	prompt = ""
	result, err = c.Ask(context.Background(), "sharedreviewquery")
	if err != nil {
		t.Fatal(err)
	}
	if prompt != "" || result.Text != "" || len(result.SourceCitations) != 0 || result.Gap == "" {
		t.Fatalf("IndexOnly world source reached provider/output: %+v %q", result, prompt)
	}
}

func TestComposeBoundedQueryExcludesUndatedClaims(t *testing.T) {
	root := reviewComposeRoot(t)
	reviewShardClaim(t, root, "undated-review", "undatedreviewquery", "account", domain.VisibilityShared, time.Time{}, "")
	var prompt string
	c := New(root, config.Default(), fakeSearchStore{}, nil, newTestRouter(&fakeProvider{modelVersion: "review@v1", sentPrompt: &prompt}), "review@v1")
	result, err := c.AskWithOptions(context.Background(), "undatedreviewquery", AskOptions{Until: time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if prompt != "" || result.Gap == "" || len(result.Citations) != 0 {
		t.Fatalf("undated claim treated as in-window: %+v prompt=%q", result, prompt)
	}
}

func TestPublicClaimsRespectsValidityAndDoesNotResurrect(t *testing.T) {
	root := reviewComposeRoot(t)
	now := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	ss := store.NewShardStore(root)
	cases := []struct{ id, from, to string }{
		{"eligible", "2026-01", "2027"},
		{"future", "2027", ""},
		{"expired", "2025", "2026-09-07"},
		{"malformed", "tomorrow", ""},
	}
	for _, tc := range cases {
		if err := ss.Append(domain.Claim{ID: tc.id, SubjectSlug: "reviewer", Predicate: "has_balance", Family: "has_balance", Object: tc.id, ObjectKey: tc.id, State: domain.StateActive, Visibility: domain.VisibilityShared, ValidFrom: tc.from, ValidTo: tc.to}); err != nil {
			t.Fatal(err)
		}
	}
	reviewShardClaim(t, root, "public-predecessor", "oldpublic", "same-account", domain.VisibilityShared, now, "")
	reviewShardClaim(t, root, "private-successor", "private replacement", "same-account", domain.VisibilityPrivate, now, "public-predecessor")
	proj, err := store.LoadMemoryProjection(store.NewSourceStore(root))
	if err != nil {
		t.Fatal(err)
	}
	got, err := PublicClaims(root, config.Default(), proj, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got["reviewer"]) != 1 || got["reviewer"][0].ID != "eligible" {
		t.Fatalf("wrong eligible live heads: %+v", got)
	}
}

package index

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
)

type sourceRecordingEmbedder struct{ texts []string }

func (*sourceRecordingEmbedder) ModelVersion() string { return "source-review@v1" }
func (e *sourceRecordingEmbedder) Embed(_ context.Context, text string) ([]float32, error) {
	e.texts = append(e.texts, text)
	return []float32{1, 0}, nil
}

func sourcePolicyIndex(t *testing.T) (string, *SQLite) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".serenity"), 0700); err != nil {
		t.Fatal(err)
	}
	eng, err := Open(filepath.Join(root, ".serenity", "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := eng.Close(); err != nil {
			t.Error(err)
		}
	})
	return root, eng
}

func TestSourcePolicyFiltersBeforeEmbeddingAndPendingCount(t *testing.T) {
	root, eng := sourcePolicyIndex(t)
	ctx := context.Background()
	ss := store.NewSourceStore(root)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	past := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	var sources []domain.Source
	for i, tc := range []struct {
		text       string
		visibility store.MemoryVisibility
		until      *time.Time
	}{
		{"public-eligible", store.MemoryVisibilityWorld, nil},
		{"PRIVATE-SHOULD-NOT-EMBED", store.MemoryVisibilityPrivate, nil},
		{"EXPIRED-SHOULD-NOT-EMBED", store.MemoryVisibilityWorld, &past},
		{"INDEXONLY-WORLD-SHOULD-NOT-EMBED", store.MemoryVisibilityWorld, nil},
	} {
		src, err := ss.WriteMemoryFact(store.MemoryFactPayload{FormatVersion: 1, RecordType: store.SourceKindMemoryFact, LegacyID: int64(i + 1), Fact: tc.text, Provenance: "review", Kind: store.MemoryFactKindFact, Visibility: tc.visibility, CreatedAt: now, ValidUntil: tc.until})
		if err != nil {
			t.Fatal(err)
		}
		sources = append(sources, src)
	}
	// Existing source metadata policy is independent of world/private visibility.
	meta := filepath.Join(ss.DirFor(sources[3].SHA256), "meta.yaml")
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
	if err := Rebuild(ctx, root, config.Default(), eng); err != nil {
		t.Fatal(err)
	}
	// Expiry can happen after indexing; stale cache rows must not escape.
	for _, src := range sources[2:3] {
		data, _, err := ss.Read(src.SHA256)
		if err != nil {
			t.Fatal(err)
		}
		rec, err := store.DecodeMemoryFact(data)
		if err != nil {
			t.Fatal(err)
		}
		if err := eng.InsertChunk(ctx, "fact:"+src.SHA256, "", rec.Fact, src.SHA256, store.SourceKindMemoryFact); err != nil {
			t.Fatal(err)
		}
	}
	if err := eng.InsertChunk(ctx, "orphan", "", "MISSING-RECORD-SHOULD-NOT-EMBED", strings.Repeat("a", 64), store.SourceKindMemoryFact); err != nil {
		t.Fatal(err)
	}
	e := &sourceRecordingEmbedder{}
	pending, err := PendingReembed(ctx, eng, e.ModelVersion())
	if err != nil {
		t.Fatal(err)
	}
	if pending != 1 {
		t.Fatalf("pending=%d, want only public eligible chunk", pending)
	}
	count, err := ReembedMissing(ctx, eng, e)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || len(e.texts) != 1 || e.texts[0] != "public-eligible" {
		t.Fatalf("restricted text reached Embed: count=%d texts=%q", count, e.texts)
	}
	proj, err := store.LoadMemoryProjection(ss)
	if err != nil {
		t.Fatal(err)
	}
	h := Hit{Kind: store.SourceKindMemoryFact, SourceSHA256: sources[3].SHA256}
	if !SourceEligibility(proj, false, false, now)(h) || SourceEligibility(proj, true, true, now)(h) {
		t.Fatal("IndexOnly confused with local visibility")
	}
}

func TestRebuildOmitsUntraceableSummaryOfPrivateHistory(t *testing.T) {
	root, eng := sourcePolicyIndex(t)
	ss := store.NewShardStore(root)
	if err := ss.Append(domain.Claim{ID: "private-review", SubjectSlug: "reviewer", Predicate: "has_balance", Family: "has_balance", Object: "PRIVATE-SUMMARY-SENTINEL", Visibility: domain.VisibilityPrivate, State: domain.StateActive}); err != nil {
		t.Fatal(err)
	}
	fw := store.NewFenceWriter(root)
	p := store.NewEntityPage(domain.Entity{Slug: "reviewer", Type: "person"})
	p.Summary = "PRIVATE-SUMMARY-SENTINEL"
	if _, err := fw.WriteEntity(p); err != nil {
		t.Fatal(err)
	}
	if err := Rebuild(context.Background(), root, config.Default(), eng); err != nil {
		t.Fatal(err)
	}
	hits, err := eng.AllChunks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if strings.Contains(h.Text, "PRIVATE-SUMMARY-SENTINEL") {
			t.Fatalf("private derived summary indexed: %+v", h)
		}
	}
	proj, err := store.LoadMemoryProjection(store.NewSourceStore(root))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	restricted, err := RestrictedSummaryEntities(root, proj, now)
	if err != nil {
		t.Fatal(err)
	}
	stale := Hit{Kind: "entity_page", EntitySlug: "reviewer", Text: "PRIVATE-SUMMARY-SENTINEL"}
	if SourceEligibility(proj, true, false, now, restricted)(stale) {
		t.Fatal("stale summary admitted by query policy")
	}
	if err := eng.InsertChunk(context.Background(), "stale-page", "reviewer", stale.Text, "", "entity_page"); err != nil {
		t.Fatal(err)
	}
	e := &sourceRecordingEmbedder{}
	if _, err := ReembedMissing(context.Background(), eng, e); err != nil {
		t.Fatal(err)
	}
	for _, text := range e.texts {
		if strings.Contains(text, "PRIVATE-SUMMARY-SENTINEL") {
			t.Fatal("stale summary reached Embed")
		}
	}
}

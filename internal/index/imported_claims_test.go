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

func importedFixture(t *testing.T, root string) *store.EntityPage {
	t.Helper()
	page := store.NewEntityPage(domain.Entity{Type: "person", Slug: "ava"})
	page.Title = "Ava"
	for _, name := range []string{"public", "private", "retracted", "expired", "future", "invalid-date"} {
		c := domain.Claim{ID: name, SubjectSlug: "ava", Predicate: "prefers", Family: "prefers", Object: name + "-import-secret", Confidence: .8, State: domain.StateActive, Visibility: domain.VisibilityShared, Review: true, SourceRef: "gbrain:people/ava#" + name, Provenance: domain.Provenance{Meta: map[string]string{"gbrain_page": "people/ava.md", "gbrain_fence": "facts"}}}
		switch name {
		case "private":
			c.Visibility = domain.VisibilityPrivate
		case "retracted":
			c.State = domain.StateRetracted
		case "expired":
			c.ValidTo = "2000-01-01"
		case "future":
			c.ValidFrom = "2999-01-01"
		case "invalid-date":
			c.ValidTo = "not-a-date"
		}
		page.Claims = append(page.Claims, c)
	}
	if _, err := store.NewFenceWriter(root).WriteEntity(page); err != nil {
		t.Fatal(err)
	}
	return page
}
func importedPolicy(t *testing.T, root string, remote, egress bool) func(Hit) bool {
	t.Helper()
	proj, err := store.LoadMemoryProjection(store.NewSourceStore(root))
	if err != nil {
		t.Fatal(err)
	}
	eligible, err := RetrievalEligibility(root, proj, remote, egress, time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return eligible
}
func importedHits(t *testing.T, eng *SQLite) []Hit {
	t.Helper()
	all, err := eng.AllChunks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var hits []Hit
	for _, h := range all {
		if h.Kind == GBrainClaimChunkKind {
			hits = append(hits, h)
		}
	}
	return hits
}

func TestImportedClaimsLocalRemoteAndEmbeddingPolicy(t *testing.T) {
	root, eng := sourcePolicyIndex(t)
	importedFixture(t, root)
	ctx := context.Background()
	if err := Rebuild(ctx, root, config.Default(), eng); err != nil {
		t.Fatal(err)
	}
	hits := importedHits(t, eng)
	if len(hits) != 2 {
		t.Fatalf("expected only current public/private rows, got %+v", hits)
	}
	local, remote := importedPolicy(t, root, false, false), importedPolicy(t, root, true, false)
	for _, h := range hits {
		if !local(h) {
			t.Fatalf("local owner cannot read %s", h.Text)
		}
		wantRemote := strings.Contains(h.Text, "public-import-secret")
		if remote(h) != wantRemote {
			t.Fatalf("remote eligibility wrong: %+v", h)
		}
		if !strings.Contains(h.Text, "review required") {
			t.Fatal("lost import-review qualifier")
		}
	}
	recorder := &sourceRecordingEmbedder{}
	n, err := ReembedMissing(ctx, eng, recorder)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 || len(recorder.texts) != 1 || !strings.Contains(recorder.texts[0], "public-import-secret") {
		t.Fatalf("private/expired data reached embedder: n=%d texts=%v", n, recorder.texts)
	}
	proj, err := store.LoadMemoryProjection(store.NewSourceStore(root))
	if err != nil {
		t.Fatal(err)
	}
	if SourceEligibility(proj, false, false, time.Now())(hits[0]) {
		t.Fatal("source-only eligibility accepted reserved claim kind without canonical validation")
	}
}

func TestImportedClaimsRejectStaleOrForgedIndexRows(t *testing.T) {
	for _, mode := range []string{"private", "changed", "deleted-row", "deleted-page", "retracted", "expired", "future", "review-changed", "forged-text", "forged-identity", "forged-source", "forged-entity", "shard-tier"} {
		t.Run(mode, func(t *testing.T) {
			root, eng := sourcePolicyIndex(t)
			page := importedFixture(t, root)
			ctx := context.Background()
			if err := Rebuild(ctx, root, config.Default(), eng); err != nil {
				t.Fatal(err)
			}
			var hit Hit
			for _, h := range importedHits(t, eng) {
				if strings.Contains(h.Text, "public-import-secret") {
					hit = h
				}
			}
			if hit.ChunkRef == "" || !importedPolicy(t, root, true, false)(hit) {
				t.Fatal("positive public control did not execute")
			}
			switch mode {
			case "private":
				page.Claims[0].Visibility = domain.VisibilityPrivate
			case "changed":
				page.Claims[0].Object = "changed canonical evidence"
			case "deleted-row":
				page.Claims = page.Claims[1:]
			case "retracted":
				page.Claims[0].State = domain.StateRetracted
			case "expired":
				page.Claims[0].ValidTo = "2000-01-01"
			case "future":
				page.Claims[0].ValidFrom = "2999-01-01"
			case "review-changed":
				page.Claims[0].Review = false
			case "forged-text":
				hit.Text = "private imported injected data"
			case "forged-identity":
				hit.ChunkRef += "-forged"
			case "forged-source":
				hit.SourceSHA256 = strings.Repeat("a", 64)
			case "forged-entity":
				hit.EntitySlug = "someone-else"
			case "shard-tier":
				cfg := config.Default()
				family := cfg.Families["prefers"]
				family.Tier = domain.TierShard
				cfg.Families["prefers"] = family
				if err := cfg.Save(filepath.Join(root, config.FileName)); err != nil {
					t.Fatal(err)
				}
			}
			path, err := store.NewFenceWriter(root).WriteEntity(page)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "deleted-page" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			if importedPolicy(t, root, true, false)(hit) || importedPolicy(t, root, true, true)(hit) {
				t.Fatalf("accepted stale or forged %s", mode)
			}
			if mode != "private" && importedPolicy(t, root, false, false)(hit) {
				t.Fatalf("local search accepted stale or forged %s", mode)
			}
		})
	}
}

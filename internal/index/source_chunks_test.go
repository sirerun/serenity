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

func TestRawSourceChunksRequireCurrentCanonicalAuthority(t *testing.T) {
	for _, mode := range []string{"deleted", "changed-bytes", "index-only", "forged-text", "forged-ref", "forged-source", "forged-kind", "forged-entity", "page-fallback"} {
		t.Run(mode, func(t *testing.T) {
			root, eng := sourcePolicyIndex(t)
			ss := store.NewSourceStore(root)
			src, err := ss.Write([]byte("Original raw source evidence."), domain.Source{Kind: "file", URI: "fixture:raw"})
			if err != nil {
				t.Fatal(err)
			}
			page := store.NewEntityPage(domain.Entity{Type: "person", Slug: "demo-person"})
			page.Title = "Demo Person"
			page.Summary = "Public summary."
			if _, err := store.NewFenceWriter(root).WriteEntity(page); err != nil {
				t.Fatal(err)
			}
			if err := Rebuild(context.Background(), root, config.Default(), eng); err != nil {
				t.Fatal(err)
			}
			chunks, err := eng.AllChunks(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			var hit Hit
			for _, h := range chunks {
				if h.SourceSHA256 == src.SHA256 {
					hit = h
				}
			}
			if hit.ChunkRef == "" || !importedPolicy(t, root, true, false)(hit) {
				t.Fatal("positive raw source control absent")
			}
			switch mode {
			case "deleted":
				if err := os.RemoveAll(ss.DirFor(src.SHA256)); err != nil {
					t.Fatal(err)
				}
			case "changed-bytes":
				if err := os.WriteFile(filepath.Join(ss.DirFor(src.SHA256), "bytes"), []byte("Tampered source bytes."), 0644); err != nil {
					t.Fatal(err)
				}
				// Source integrity already fails the request closed before policy.
				if _, err := store.LoadMemoryProjection(ss); err == nil || !strings.Contains(err.Error(), "hash mismatch") {
					t.Fatalf("changed source must fail integrity validation: %v", err)
				}
				return
			case "index-only":
				path := filepath.Join(ss.DirFor(src.SHA256), "meta.yaml")
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, append(raw, []byte("index_only: true\n")...), 0644); err != nil {
					t.Fatal(err)
				}
			case "forged-text":
				hit.Text = "Forged raw evidence."
			case "forged-ref":
				hit.ChunkRef += "-forged"
			case "forged-source":
				hit.SourceSHA256 = ""
			case "forged-kind":
				hit.Kind = "unknown"
			case "forged-entity":
				hit.EntitySlug = "someone-else"
			case "page-fallback":
				hit.Kind = "entity_page"
				hit.ChunkRef = "page:demo-person"
				hit.SourceSHA256 = ""
				hit.EntitySlug = "demo-person"
			}
			if importedPolicy(t, root, true, false)(hit) || importedPolicy(t, root, false, true)(hit) {
				t.Fatalf("stale/forged %s retained disclosure authority", mode)
			}
			if local := importedPolicy(t, root, false, false)(hit); local != (mode == "index-only") {
				t.Fatalf("local canonical eligibility=%v for %s", local, mode)
			}
		})
	}
}

func TestMemorySourceChunkFieldsRequireCanonicalAuthority(t *testing.T) {
	root, eng := sourcePolicyIndex(t)
	src, err := store.NewSourceStore(root).WriteMemoryFact(store.MemoryFactPayload{FormatVersion: 1, RecordType: store.SourceKindMemoryFact, LegacyID: 1, Fact: "Verified memory source.", EntitySlug: "demo-person", Provenance: "fixture", Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityWorld, CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := Rebuild(context.Background(), root, config.Default(), eng); err != nil {
		t.Fatal(err)
	}
	valid := Hit{ChunkRef: "fact:" + src.SHA256, Text: "Verified memory source.", EntitySlug: "demo-person", SourceSHA256: src.SHA256, Kind: store.SourceKindMemoryFact}
	policy := importedPolicy(t, root, true, true)
	if !policy(valid) {
		t.Fatal("positive canonical memory source missing")
	}
	for _, field := range []string{"text", "entity", "kind", "ref", "source"} {
		t.Run(field, func(t *testing.T) {
			h := valid
			switch field {
			case "text":
				h.Text = "Forged memory"
			case "entity":
				h.EntitySlug = "someone-else"
			case "kind":
				h.Kind = "file"
			case "ref":
				h.ChunkRef = "src:" + src.SHA256 + ":0"
			case "source":
				h.SourceSHA256 = ""
			}
			if policy(h) {
				t.Fatal("forged memory projection retained authority")
			}
		})
	}
}

func TestEntityPageChunkRechecksCurrentContent(t *testing.T) {
	for _, mode := range []string{"edited", "deleted", "forged-text", "forged-source"} {
		t.Run(mode, func(t *testing.T) {
			root, eng := sourcePolicyIndex(t)
			fw := store.NewFenceWriter(root)
			page := store.NewEntityPage(domain.Entity{Type: "person", Slug: "demo-person"})
			page.Title, page.Summary = "Demo Person", "Original public summary."
			path, err := fw.WriteEntity(page)
			if err != nil {
				t.Fatal(err)
			}
			if err := Rebuild(context.Background(), root, config.Default(), eng); err != nil {
				t.Fatal(err)
			}
			h := Hit{ChunkRef: "page:demo-person", EntitySlug: "demo-person", Kind: "entity_page", Text: page.Title + "\n" + page.Summary}
			if !importedPolicy(t, root, true, true)(h) {
				t.Fatal("positive page control absent")
			}
			switch mode {
			case "edited":
				page.Summary = "Current public summary."
				if _, err := fw.WriteEntity(page); err != nil {
					t.Fatal(err)
				}
			case "deleted":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "forged-text":
				h.Text = "Forged summary."
			case "forged-source":
				h.SourceSHA256 = strings.Repeat("a", 64)
			}
			if importedPolicy(t, root, true, true)(h) {
				t.Fatal("stale page retained authority")
			}
		})
	}
}

func TestRawSourceEligibilityPreventsEmbeddingEgress(t *testing.T) {
	for _, mode := range []string{"deleted", "index-only", "forged-text"} {
		t.Run(mode, func(t *testing.T) {
			root, eng := sourcePolicyIndex(t)
			ctx := context.Background()
			ss := store.NewSourceStore(root)
			src, err := ss.Write([]byte("Original embedding evidence."), domain.Source{Kind: "file", URI: "fixture:embed"})
			if err != nil {
				t.Fatal(err)
			}
			if err := Rebuild(ctx, root, config.Default(), eng); err != nil {
				t.Fatal(err)
			}
			positive := &sourceRecordingEmbedder{}
			if n, err := ReembedMissing(ctx, eng, positive); err != nil || n != 1 || len(positive.texts) != 1 || positive.texts[0] != "Original embedding evidence." {
				t.Fatalf("positive embedding control: n=%d err=%v texts=%q", n, err, positive.texts)
			}
			switch mode {
			case "deleted":
				if err := os.RemoveAll(ss.DirFor(src.SHA256)); err != nil {
					t.Fatal(err)
				}
			case "index-only":
				path := filepath.Join(ss.DirFor(src.SHA256), "meta.yaml")
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, append(raw, []byte("index_only: true\n")...), 0644); err != nil {
					t.Fatal(err)
				}
			case "forged-text":
				all, err := eng.AllChunks(ctx)
				if err != nil {
					t.Fatal(err)
				}
				h := all[0]
				if _, err := eng.db.ExecContext(ctx, `UPDATE chunks SET text = ? WHERE chunk_ref = ?`, "FORGED-PROVIDER-CONTENT", h.ChunkRef); err != nil {
					t.Fatal(err)
				}
			}
			// A different pin must reconsider current authority before spending a call.
			provider := &fakeEmbedder{pin: "new-authority-check@v1"}
			if n, err := PendingReembed(ctx, eng, provider.pin); err != nil || n != 0 {
				t.Fatalf("ineligible pending=%d err=%v", n, err)
			}
			if n, err := ReembedMissing(ctx, eng, provider); err != nil || n != 0 || provider.calls != 0 {
				t.Fatalf("ineligible provider egress: n=%d calls=%d err=%v", n, provider.calls, err)
			}
		})
	}
}

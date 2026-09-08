package index

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
)

func TestNativeHumanClaimTextIsSearchable(t *testing.T) {
	root, eng := sourcePolicyIndex(t)
	fw := store.NewFenceWriter(root)
	page := store.NewEntityPage(domain.Entity{Type: "person", Slug: "demo-person"})
	page.Title = "Demo Person"
	page.Claims = []domain.Claim{{ID: "human-claim", SubjectSlug: "demo-person", Predicate: "works_at", Family: "works_at", Object: "Blue Heron Workshop", Confidence: 1, State: domain.StateActive, Provenance: domain.Provenance{Actor: "human:reviewer"}}}
	if _, err := fw.WriteEntity(page); err != nil {
		t.Fatal(err)
	}
	if err := Rebuild(context.Background(), root, config.Default(), eng); err != nil {
		t.Fatal(err)
	}
	hits, err := eng.SearchFTS(context.Background(), LiteralFTSQuery("Blue Heron Workshop"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("human claim is not searchable: %+v", hits)
	}
	if !importedPolicy(t, root, true, false)(hits[0]) {
		t.Fatal("eligible human claim absent from remote recall")
	}
}

func nativePolicyFixture(t *testing.T, root, family string) ([]domain.Claim, func([]domain.Claim), string) {
	t.Helper()
	ss := store.NewSourceStore(root)
	public, err := ss.Write([]byte("Public original source."), domain.Source{Kind: "file", URI: "fixture:public"})
	if err != nil {
		t.Fatal(err)
	}
	restricted, err := ss.Write([]byte("Private index-only source."), domain.Source{Kind: "file", URI: "fixture:index-only", IndexOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	rows := []domain.Claim{}
	for _, name := range []string{"public", "private", "indexonly", "dangling", "legacy", "retracted", "expired", "future", "predecessor", "successor"} {
		c := domain.Claim{ID: name, SubjectSlug: "demo-person", Predicate: family, Family: family, Object: name + "-native-evidence", ObjectKey: name, Confidence: .9, State: domain.StateActive, Visibility: domain.VisibilityShared, Provenance: domain.Provenance{Actor: "human:fixture"}}
		switch name {
		case "public":
			c.Provenance.SourceSHA256 = public.SHA256
			c.Provenance.Model = "fixture@v1"
		case "private":
			c.Visibility = domain.VisibilityPrivate
		case "indexonly":
			c.Provenance.SourceSHA256 = restricted.SHA256
		case "dangling":
			c.Provenance.SourceSHA256 = strings.Repeat("b", 64)
		case "legacy":
			c.Visibility = ""
		case "retracted":
			c.State = domain.StateRetracted
		case "expired":
			c.ValidTo = "2000-01-01"
		case "future":
			c.ValidFrom = "2999-01-01"
		case "successor":
			c.Supersedes = "predecessor"
			c.Visibility = domain.VisibilityPrivate
		}
		rows = append(rows, c)
	}
	if config.Default().TierOf(family) == domain.TierFence {
		rows[8].SupersededBy = "successor"
		rows[8].State = domain.StateSuperseded
	}
	write := func(rows []domain.Claim) {
		t.Helper()
		if config.Default().TierOf(family) == domain.TierShard {
			path := store.NewShardStore(root).PathFor("demo-person", family)
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			var raw bytes.Buffer
			for _, c := range rows {
				if err := json.NewEncoder(&raw).Encode(c); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(path, raw.Bytes(), 0644); err != nil {
				t.Fatal(err)
			}
		} else {
			page := store.NewEntityPage(domain.Entity{Type: "person", Slug: "demo-person"})
			page.Title = "Demo Person"
			page.Claims = rows
			if _, err := store.NewFenceWriter(root).WriteEntity(page); err != nil {
				t.Fatal(err)
			}
		}
	}
	write(rows)
	return rows, write, public.SHA256
}

func nativeHits(t *testing.T, eng *SQLite) []Hit {
	t.Helper()
	all, err := eng.AllChunks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out []Hit
	for _, h := range all {
		if h.Kind == CanonicalClaimChunkKind {
			out = append(out, h)
		}
	}
	return out
}

func TestNativeClaimLocalRemoteAndProviderPolicy(t *testing.T) {
	for _, family := range []string{"works_at", "has_balance"} {
		t.Run(family, func(t *testing.T) {
			root, eng := sourcePolicyIndex(t)
			nativePolicyFixture(t, root, family)
			ctx := context.Background()
			if err := Rebuild(ctx, root, config.Default(), eng); err != nil {
				t.Fatal(err)
			}
			before, err := DumpString(ctx, eng)
			if err != nil {
				t.Fatal(err)
			}
			if err := Rebuild(ctx, root, config.Default(), eng); err != nil {
				t.Fatal(err)
			}
			after, err := DumpString(ctx, eng)
			if err != nil || before != after {
				t.Fatalf("derived projection not deterministic: %v", err)
			}
			hits := nativeHits(t, eng)
			if len(hits) != 6 {
				t.Fatalf("canonical active count=%d: %+v", len(hits), hits)
			}
			local, remote, egress := importedPolicy(t, root, false, false), importedPolicy(t, root, true, false), importedPolicy(t, root, false, true)
			for _, h := range hits {
				if !local(h) {
					t.Fatalf("local owner cannot inspect canonical claim: %+v", h)
				}
				public := strings.Contains(h.Text, "public-native-evidence") || strings.Contains(h.Text, "legacy-native-evidence")
				if remote(h) != public || egress(h) != public {
					t.Fatalf("remote/provider policy: %+v", h)
				}
			}
			recorder := &sourceRecordingEmbedder{}
			if _, err := ReembedMissing(ctx, eng, recorder); err != nil {
				t.Fatal(err)
			}
			nativeCount := 0
			for _, text := range recorder.texts {
				if strings.Contains(text, "[canonical claim:") {
					nativeCount++
					if !strings.Contains(text, "public-native-evidence") && !strings.Contains(text, "legacy-native-evidence") {
						t.Fatalf("restricted native claim sent to provider: %s", text)
					}
				}
			}
			if nativeCount != 2 {
				t.Fatalf("eligible native embeddings=%d", nativeCount)
			}
			proj, err := store.LoadMemoryProjection(store.NewSourceStore(root))
			if err != nil {
				t.Fatal(err)
			}
			if SourceEligibility(proj, false, false, time.Now())(hits[0]) {
				t.Fatal("source-only policy accepted a canonical projection")
			}
		})
	}
}

func TestNativeClaimRejectsStaleOrForgedProjection(t *testing.T) {
	for _, family := range []string{"works_at", "has_balance"} {
		t.Run(family, func(t *testing.T) {
			for _, mode := range []string{"private", "changed", "confidence", "actor", "retracted", "expired", "future", "deleted-row", "private-successor", "deleted-source", "source-indexonly", "forged-text", "forged-ref", "forged-kind", "forged-source", "forged-entity"} {
				t.Run(mode, func(t *testing.T) {
					root, eng := sourcePolicyIndex(t)
					rows, write, source := nativePolicyFixture(t, root, family)
					ctx := context.Background()
					if err := Rebuild(ctx, root, config.Default(), eng); err != nil {
						t.Fatal(err)
					}
					var hit Hit
					for _, h := range nativeHits(t, eng) {
						if strings.Contains(h.Text, "public-native-evidence") {
							hit = h
						}
					}
					if hit.ChunkRef == "" || !importedPolicy(t, root, true, false)(hit) {
						t.Fatal("positive control absent")
					}
					switch mode {
					case "private":
						rows[0].Visibility = domain.VisibilityPrivate
					case "changed":
						rows[0].Object = "new canonical value"
					case "confidence":
						rows[0].Confidence = .7
					case "actor":
						rows[0].Provenance.Actor = "human:changed"
					case "retracted":
						rows[0].State = domain.StateRetracted
					case "expired":
						rows[0].ValidTo = "2000-01-01"
					case "future":
						rows[0].ValidFrom = "2999-01-01"
					case "deleted-row":
						rows = rows[1:]
					case "private-successor":
						c := rows[0]
						c.ID = "new-private"
						c.Object = "private replacement"
						c.Supersedes = rows[0].ID
						c.Visibility = domain.VisibilityPrivate
						if config.Default().TierOf(family) == domain.TierFence {
							rows[0].SupersededBy = c.ID
							rows[0].State = domain.StateSuperseded
						}
						rows = append(rows, c)
					case "deleted-source":
						if err := os.RemoveAll(store.NewSourceStore(root).DirFor(source)); err != nil {
							t.Fatal(err)
						}
					case "source-indexonly":
						path := filepath.Join(store.NewSourceStore(root).DirFor(source), "meta.yaml")
						raw, err := os.ReadFile(path)
						if err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(path, append(raw, []byte("index_only: true\n")...), 0644); err != nil {
							t.Fatal(err)
						}
					case "forged-text":
						hit.Text = "forged canonical evidence"
					case "forged-ref":
						hit.ChunkRef += "-forged"
					case "forged-kind":
						hit.Kind = "file"
					case "forged-source":
						hit.SourceSHA256 = ""
					case "forged-entity":
						hit.EntitySlug = "someone-else"
					}
					write(rows)
					if importedPolicy(t, root, true, false)(hit) || importedPolicy(t, root, false, true)(hit) {
						t.Fatalf("stale or forged %s passed", mode)
					}
				})
			}
		})
	}
}

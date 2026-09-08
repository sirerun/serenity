package memory

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/store"
)

func TestRecallNativeHumanClaimRechecksCanonicalPrivacy(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()
	page := store.NewEntityPage(domain.Entity{Type: "person", Slug: "demo-person"})
	page.Title = "Demo Person"
	page.Claims = []domain.Claim{{ID: "human-assertion", SubjectSlug: "demo-person", Predicate: "works_at", Family: "works_at", Object: "Blue Heron Workshop", State: domain.StateActive, Confidence: 1, Provenance: domain.Provenance{Actor: "human:reviewer"}}}
	if _, err := h.deps.Fence.WriteEntity(page); err != nil {
		t.Fatal(err)
	}
	if err := index.Rebuild(ctx, h.deps.Root, h.deps.Config, h.deps.Index); err != nil {
		t.Fatal(err)
	}
	recall := func() recallResponse {
		t.Helper()
		value, isError, err := h.recall(ctx, mustMarshal(t, recallRequest{Query: "Blue Heron Workshop"}))
		if err != nil || isError {
			t.Fatalf("recall: %v %+v", err, value)
		}
		return value.(recallResponse)
	}
	result := recall()
	if len(result.Results) != 1 || result.Results[0].Chunk == nil || !strings.Contains(*result.Results[0].Chunk, "human:reviewer") || result.Results[0].Slug != "demo-person" {
		t.Fatalf("native assertion absent or attribution lost: %+v", result)
	}
	page.Claims[0].Visibility = domain.VisibilityPrivate
	if _, err := h.deps.Fence.WriteEntity(page); err != nil {
		t.Fatal(err)
	}
	if stale := recall(); len(stale.Results) != 0 {
		t.Fatalf("stale index leaked newly private assertion: %+v", stale)
	}
}

func TestRecallRawSourceRechecksDeletionAndIndexOnly(t *testing.T) {
	for _, mode := range []string{"deleted", "index-only"} {
		t.Run(mode, func(t *testing.T) {
			h, _ := newTestHandlers(t)
			ss := store.NewSourceStore(h.deps.Root)
			src, err := ss.Write([]byte("Blue Heron source evidence."), domain.Source{Kind: "file", URI: "fixture:source"})
			if err != nil {
				t.Fatal(err)
			}
			if err := index.Rebuild(context.Background(), h.deps.Root, h.deps.Config, h.deps.Index); err != nil {
				t.Fatal(err)
			}
			recall := func() recallResponse {
				t.Helper()
				value, isError, err := h.recall(context.Background(), mustMarshal(t, recallRequest{Query: "Blue Heron"}))
				if err != nil || isError {
					t.Fatalf("recall: %v %+v", err, value)
				}
				return value.(recallResponse)
			}
			if got := recall(); len(got.Results) != 1 || got.Results[0].Chunk == nil || *got.Results[0].Chunk != "Blue Heron source evidence." {
				t.Fatalf("positive raw-source control absent: %+v", got)
			}
			if mode == "deleted" {
				if err := os.RemoveAll(ss.DirFor(src.SHA256)); err != nil {
					t.Fatal(err)
				}
			} else {
				path := filepath.Join(ss.DirFor(src.SHA256), "meta.yaml")
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, append(raw, []byte("index_only: true\n")...), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if got := recall(); len(got.Results) != 0 {
				t.Fatalf("stale raw source disclosed: %+v", got)
			}
		})
	}
}

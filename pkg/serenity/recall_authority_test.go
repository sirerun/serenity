package serenity_test

import (
	"context"
	"os"
	"testing"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/pkg/serenity"
)

func TestEmbeddedRecallRechecksCanonicalAuthority(t *testing.T) {
	for _, kind := range []string{"raw-source", "native-claim"} {
		t.Run(kind, func(t *testing.T) {
			root := fixtureBrain(t)
			var remove string
			if kind == "raw-source" {
				ss := store.NewSourceStore(root)
				src, err := ss.Write([]byte("Blue Heron evidence."), domain.Source{Kind: "file", URI: "fixture:facade"})
				if err != nil {
					t.Fatal(err)
				}
				remove = ss.DirFor(src.SHA256)
			} else {
				page := store.NewEntityPage(domain.Entity{Type: "person", Slug: "demo-person"})
				page.Claims = []domain.Claim{{ID: "human", SubjectSlug: "demo-person", Predicate: "works_at", Family: "works_at", Object: "Blue Heron", Confidence: 1, State: domain.StateActive, Visibility: domain.VisibilityPrivate, Provenance: domain.Provenance{Actor: "human:fixture"}}}
				var err error
				remove, err = store.NewFenceWriter(root).WriteEntity(page)
				if err != nil {
					t.Fatal(err)
				}
			}
			eng, err := providers.OpenIndex(root)
			if err != nil {
				t.Fatal(err)
			}
			if err := index.Rebuild(context.Background(), root, config.Default(), eng); err != nil {
				t.Fatal(err)
			}
			if err := eng.Close(); err != nil {
				t.Fatal(err)
			}
			brain, err := serenity.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			before, err := brain.Recall(context.Background(), "Blue Heron", serenity.Budget{})
			if err != nil || len(before.Hits) != 1 {
				t.Fatalf("local-owner positive control: %+v %v", before, err)
			}
			if err := os.RemoveAll(remove); err != nil {
				t.Fatal(err)
			}
			after, err := brain.Recall(context.Background(), "Blue Heron", serenity.Budget{})
			if err != nil || len(after.Hits) != 0 {
				t.Fatalf("deleted canonical evidence retained facade authority: %+v %v", after, err)
			}
		})
	}
}

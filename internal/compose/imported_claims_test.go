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

func TestComposeImportedClaimsPreservesReviewAndEgressPolicy(t *testing.T) {
	root := reviewComposeRoot(t)
	cfg := config.Default()
	page := store.NewEntityPage(domain.Entity{Type: "person", Slug: "ava"})
	page.Title = "Ava"
	for _, name := range []string{"public", "private"} {
		visibility := domain.VisibilityShared
		if name == "private" {
			visibility = domain.VisibilityPrivate
		}
		page.Claims = append(page.Claims, domain.Claim{ID: name, SubjectSlug: "ava", Predicate: "prefers", Family: "prefers", Object: name + " importedreviewquery", ObjectKey: name, Confidence: .8, Visibility: visibility, State: domain.StateActive, Review: true, SourceRef: "gbrain:ava#" + name, Provenance: domain.Provenance{Meta: map[string]string{"gbrain_page": "people/ava.md", "gbrain_fence": "facts"}}})
	}
	if _, err := store.NewFenceWriter(root).WriteEntity(page); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".serenity"), 0700); err != nil {
		t.Fatal(err)
	}
	eng, err := index.Open(filepath.Join(root, ".serenity", "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()
	ctx := context.Background()
	if err := index.Rebuild(ctx, root, cfg, eng); err != nil {
		t.Fatal(err)
	}
	var prompt string
	provider := &fakeProvider{modelVersion: "review@v1", resp: router.Response{Text: "Imported evidence [claim:public]."}, sentPrompt: &prompt}
	c := New(root, cfg, eng, nil, newTestRouter(provider), "review@v1")
	result, err := c.AskWithOptions(ctx, "importedreviewquery", AskOptions{Now: time.Date(2026, 9, 8, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "public importedreviewquery") || strings.Contains(prompt, "private importedreviewquery") || !strings.Contains(prompt, "requiring human review") || len(result.Citations) != 1 {
		t.Fatalf("wrong imported evidence at provider: %q %+v", prompt, result)
	}
}

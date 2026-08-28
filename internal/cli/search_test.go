package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// TestSearchCLIFindsEntityPageChunkAfterSync drives `serenity search`
// through the CLI path (T1.11): a fence entity page written and synced
// must be findable by a word from its summary, with the no-embedder
// degraded-mode notice stated plainly.
func TestSearchCLIFindsEntityPageChunkAfterSync(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	var out bytes.Buffer
	if err := runInit(root, &out); err != nil {
		t.Fatal(err)
	}

	q := writer.NewQueue(nil)
	defer q.Close()

	fw := store.NewFenceWriter(root)
	p := store.NewEntityPage(domain.Entity{Type: "person", Slug: "alice-tan"})
	p.Summary = "Runs engineering at Acme, leading the quasar rollout."
	if _, _, err := writer.Fence(q, fw, p); err != nil {
		t.Fatal(err)
	}

	if err := runSync(ctx, root, &out); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := runSearch(ctx, root, "quasar", 10, &out); err != nil {
		t.Fatal(err)
	}
	result := out.String()
	if !strings.Contains(result, "no embedding model pinned; running full-text-only search") {
		t.Fatalf("expected the degraded-mode notice, got:\n%s", result)
	}
	if !strings.Contains(result, "page:alice-tan") {
		t.Fatalf("expected the alice-tan entity page chunk in results, got:\n%s", result)
	}
}

// TestSearchCLINoResults proves an unmatched query reports plainly rather
// than erroring.
func TestSearchCLINoResults(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	var out bytes.Buffer
	if err := runInit(root, &out); err != nil {
		t.Fatal(err)
	}
	if err := runSync(ctx, root, &out); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := runSearch(ctx, root, "nonexistentterm", 10, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no results") {
		t.Fatalf("expected \"no results\", got:\n%s", out.String())
	}
}

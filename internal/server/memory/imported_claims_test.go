package memory

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/import/gbrain"
	"github.com/sirerun/serenity/internal/index"
)

func TestRecallImportedClaimsKeepsPrivacyAfterCanonicalEdit(t *testing.T) {
	h, root := newTestHandlers(t)
	ctx := context.Background()
	if _, err := gbrain.Import(ctx, "../../../testdata/gbrain-fixture", root, h.deps.Config); err != nil {
		t.Fatal(err)
	}
	if err := index.Rebuild(ctx, root, h.deps.Config, h.deps.Index); err != nil {
		t.Fatal(err)
	}
	recall := func(query string) recallResponse {
		t.Helper()
		resp, isError, err := h.recall(ctx, mustMarshal(t, recallRequest{Query: query}))
		if err != nil || isError {
			t.Fatalf("recall: %v %+v", err, resp)
		}
		return resp.(recallResponse)
	}
	public := recall("Synthetic project")
	if len(public.Results) != 1 || public.Results[0].Slug != "beacon" || public.Results[0].Chunk == nil || !strings.Contains(*public.Results[0].Chunk, "review required") {
		t.Fatalf("public imported row missing: %+v", public)
	}
	if private := recall("feature flags"); len(private.Results) != 0 {
		t.Fatalf("private imported preference leaked: %+v", private)
	}
	path := filepath.Join(root, "brain", "entities", "project", "beacon.md")
	page, err := h.deps.Fence.ParseEntity(path)
	if err != nil {
		t.Fatal(err)
	}
	page.Claims[0].Visibility = domain.VisibilityPrivate
	if _, err := h.deps.Fence.WriteEntity(page); err != nil {
		t.Fatal(err)
	}
	if stale := recall("Synthetic project"); len(stale.Results) != 0 {
		t.Fatalf("stale index leaked newly private claim: %+v", stale)
	}
}

package entities

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
)

func TestSplitPartitionsClaimsIntoTwoPages(t *testing.T) {
	r := newTestRig(t)

	orig := domain.Entity{Type: "person", Slug: "acme-corp"}
	origPage := store.NewEntityPage(orig)
	origPage.Claims = []domain.Claim{
		fenceClaim("c-1", orig.Slug, "works_at", "acme"),
		fenceClaim("c-2", orig.Slug, "has_role", "engineer"),
		fenceClaim("c-3", orig.Slug, "prefers", "async standups"),
		fenceClaim("c-4", orig.Slug, "committed_to", "security review"),
	}
	r.seedPage(t, origPage)

	target := domain.Entity{Type: "person", Slug: "acme-labs"}

	res, err := Split(r.q, r.fw, orig, target, []string{"c-3", "c-4"}, mergeFixedNow)
	if err != nil {
		t.Fatalf("Split: %v", err)
	}
	if res.Moved != 2 {
		t.Fatalf("Moved = %d, want 2", res.Moved)
	}

	origAfter, err := r.fw.ParseEntity(res.OriginalPath)
	if err != nil {
		t.Fatalf("parse original: %v", err)
	}
	if len(origAfter.Claims) != 2 {
		t.Fatalf("original page has %d claims, want 2", len(origAfter.Claims))
	}
	origIDs := map[string]bool{}
	for _, c := range origAfter.Claims {
		origIDs[c.ID] = true
		if c.SubjectSlug != orig.Slug {
			t.Fatalf("claim %s: SubjectSlug = %q, want %q (unmoved claim)", c.ID, c.SubjectSlug, orig.Slug)
		}
	}
	if !origIDs["c-1"] || !origIDs["c-2"] {
		t.Fatalf("original page claims = %v, want c-1 and c-2", origIDs)
	}

	newPage, err := r.fw.ParseEntity(res.NewPath)
	if err != nil {
		t.Fatalf("parse new page: %v", err)
	}
	if len(newPage.Claims) != 2 {
		t.Fatalf("new page has %d claims, want 2", len(newPage.Claims))
	}
	newIDs := map[string]bool{}
	for _, c := range newPage.Claims {
		newIDs[c.ID] = true
		if c.SubjectSlug != target.Slug {
			t.Fatalf("claim %s: SubjectSlug = %q, want %q (moved claim)", c.ID, c.SubjectSlug, target.Slug)
		}
	}
	if !newIDs["c-3"] || !newIDs["c-4"] {
		t.Fatalf("new page claims = %v, want c-3 and c-4", newIDs)
	}
}

func TestSplitMissingClaimIDRefusesAndWritesNothing(t *testing.T) {
	r := newTestRig(t)

	orig := domain.Entity{Type: "person", Slug: "acme-corp"}
	origPage := store.NewEntityPage(orig)
	origPage.Claims = []domain.Claim{fenceClaim("c-1", orig.Slug, "works_at", "acme")}
	origPath := r.seedPage(t, origPage)
	before, err := os.ReadFile(origPath)
	if err != nil {
		t.Fatal(err)
	}

	target := domain.Entity{Type: "person", Slug: "acme-labs"}
	_, err = Split(r.q, r.fw, orig, target, []string{"c-1", "does-not-exist"}, mergeFixedNow)
	if !errors.Is(err, ErrClaimNotFound) {
		t.Fatalf("err = %v, want ErrClaimNotFound", err)
	}

	after, _ := os.ReadFile(origPath)
	if !bytes.Equal(before, after) {
		t.Fatal("original page was modified despite Split refusing")
	}
	newPath := r.fw.PathFor(target.Type, target.Slug)
	if _, err := os.Stat(newPath); !os.IsNotExist(err) {
		t.Fatal("new page must not exist when Split refuses")
	}
}

func TestSplitRefusesTypeMismatch(t *testing.T) {
	r := newTestRig(t)
	orig := domain.Entity{Type: "person", Slug: "acme-corp"}
	r.seedPage(t, store.NewEntityPage(orig))
	target := domain.Entity{Type: "company", Slug: "acme-labs"}
	_, err := Split(r.q, r.fw, orig, target, []string{"c-1"}, mergeFixedNow)
	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("err = %v, want ErrTypeMismatch", err)
	}
}

func TestSplitEmptyMoveIDsRefuses(t *testing.T) {
	r := newTestRig(t)
	orig := domain.Entity{Type: "person", Slug: "acme-corp"}
	r.seedPage(t, store.NewEntityPage(orig))
	target := domain.Entity{Type: "person", Slug: "acme-labs"}
	if _, err := Split(r.q, r.fw, orig, target, nil, mergeFixedNow); err == nil {
		t.Fatal("want an error for empty moveIDs")
	}
}

// TestSplitThenMergeRoundTrips proves split's inverse is Merge: splitting
// an entity in two and merging the two halves back together restores the
// original claim set (RFC §10.5 pairs "auto-merge" with "splits supported
// from the client" as the two identity-editing primitives).
func TestSplitThenMergeRoundTrips(t *testing.T) {
	r := newTestRig(t)

	orig := domain.Entity{Type: "person", Slug: "acme-corp"}
	origPage := store.NewEntityPage(orig)
	origPage.Claims = []domain.Claim{
		fenceClaim("c-1", orig.Slug, "works_at", "acme"),
		fenceClaim("c-2", orig.Slug, "has_role", "engineer"),
	}
	r.seedPage(t, origPage)

	target := domain.Entity{Type: "person", Slug: "acme-labs"}
	if _, err := Split(r.q, r.fw, orig, target, []string{"c-2"}, mergeFixedNow); err != nil {
		t.Fatalf("Split: %v", err)
	}

	if _, err := Merge(r.q, r.fw, r.ss, orig, target, mergeFixedNow, "human:david"); err != nil {
		t.Fatalf("Merge back: %v", err)
	}

	merged, err := r.fw.ParseEntity(r.fw.PathFor(orig.Type, orig.Slug))
	if err != nil {
		t.Fatalf("parse remerged page: %v", err)
	}
	if len(merged.Claims) != 2 {
		t.Fatalf("remerged page has %d claims, want 2", len(merged.Claims))
	}
}

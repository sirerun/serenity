package entities

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

var mergeFixedNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

// testRig bundles the fixtures Merge/Split/Undo need over one temp brain
// root, mirroring internal/supersede's own temp-dir test setup.
type testRig struct {
	root string
	fw   *store.FenceWriter
	ss   *store.ShardStore
	q    *writer.Queue
}

func newTestRig(t *testing.T) *testRig {
	t.Helper()
	root := t.TempDir()
	q := writer.NewQueue(nil)
	t.Cleanup(q.Close)
	return &testRig{root: root, fw: store.NewFenceWriter(root), ss: store.NewShardStore(root), q: q}
}

func fenceClaim(id, subject, predicate, object string) domain.Claim {
	return domain.Claim{
		ID: id, SubjectSlug: subject, Predicate: predicate, Object: object,
		ObjectKey: store.NormalizeKey(object), Confidence: 0.9, State: domain.StateActive,
		Family: predicate, SourceRef: "e1#1",
	}
}

func (r *testRig) seedPage(t *testing.T, p *store.EntityPage) string {
	t.Helper()
	path, _, err := writer.Fence(r.q, r.fw, p)
	if err != nil {
		t.Fatalf("seed page %s: %v", path, err)
	}
	return path
}

func TestMergeRepointsEveryClaimAndRendersOnePage(t *testing.T) {
	r := newTestRig(t)

	a := domain.Entity{Type: "person", Slug: "acme-corp", Aliases: []string{"Acme"}}
	b := domain.Entity{Type: "person", Slug: "acme-corporation", Aliases: []string{"Acme Corp Inc"}}

	aPage := store.NewEntityPage(a)
	aPage.Claims = []domain.Claim{
		fenceClaim("c-a1", a.Slug, "works_at", "acme"),
		fenceClaim("c-a2", a.Slug, "has_role", "engineer"),
	}
	r.seedPage(t, aPage)

	bPage := store.NewEntityPage(b)
	bPage.Claims = []domain.Claim{
		fenceClaim("c-b1", b.Slug, "prefers", "async standups"),
		fenceClaim("c-b2", b.Slug, "committed_to", "security review"),
	}
	bPath := r.seedPage(t, bPage)

	ev, err := Merge(r.q, r.fw, r.ss, a, b, mergeFixedNow, "human:david")
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if ev.ClaimsMoved != 2 {
		t.Fatalf("ClaimsMoved = %d, want 2", ev.ClaimsMoved)
	}

	// b's page must be gone -- "renders one page".
	if _, err := os.Stat(bPath); !os.IsNotExist(err) {
		t.Fatalf("b's page still exists after merge (err=%v)", err)
	}

	// a's page must render successfully and hold every claim, each
	// re-pointed to a's slug.
	merged, err := r.fw.ParseEntity(ev.APath)
	if err != nil {
		t.Fatalf("parse merged page: %v", err)
	}
	if len(merged.Claims) != 4 {
		t.Fatalf("merged page has %d claims, want 4 (2+2)", len(merged.Claims))
	}
	seen := map[string]bool{}
	for _, c := range merged.Claims {
		seen[c.ID] = true
		if c.SubjectSlug != a.Slug {
			t.Fatalf("claim %s: SubjectSlug = %q, want %q", c.ID, c.SubjectSlug, a.Slug)
		}
	}
	for _, want := range []string{"c-a1", "c-a2", "c-b1", "c-b2"} {
		if !seen[want] {
			t.Fatalf("merged page missing claim %s", want)
		}
	}

	// b's slug and its own alias become aliases of a.
	aliasSet := map[string]bool{}
	for _, al := range merged.Entity.Aliases {
		aliasSet[normalizeName(al)] = true
	}
	for _, want := range []string{"acme", normalizeName(b.Slug), normalizeName("Acme Corp Inc")} {
		if !aliasSet[want] {
			t.Fatalf("merged aliases %v missing %q", merged.Entity.Aliases, want)
		}
	}

	// The deterministic-writer round-trip invariant (§7.2) must still
	// hold post-merge: re-rendering the parsed page reproduces the exact
	// bytes on disk.
	onDisk, err := os.ReadFile(ev.APath)
	if err != nil {
		t.Fatalf("read %s: %v", ev.APath, err)
	}
	rerendered, err := r.fw.RenderEntity(merged)
	if err != nil {
		t.Fatalf("re-render merged page: %v", err)
	}
	if !bytes.Equal(onDisk, rerendered) {
		t.Fatalf("merged page does not round-trip byte-identically:\n--- disk ---\n%s\n--- re-rendered ---\n%s", onDisk, rerendered)
	}
}

func TestMergeRefusesShardTierAbsorbedEntity(t *testing.T) {
	r := newTestRig(t)

	a := domain.Entity{Type: "person", Slug: "acme-corp"}
	b := domain.Entity{Type: "person", Slug: "acme-corporation"}
	r.seedPage(t, store.NewEntityPage(a))
	bPath := r.seedPage(t, store.NewEntityPage(b))
	bBefore, err := os.ReadFile(bPath)
	if err != nil {
		t.Fatal(err)
	}

	// Give b a shard-tier claim family.
	if err := r.ss.Append(domain.Claim{
		SubjectSlug: b.Slug, Predicate: "has_balance", Object: "$100",
		Family: "has_balance", State: domain.StateActive, Confidence: 0.9,
	}); err != nil {
		t.Fatalf("seed shard claim: %v", err)
	}

	aPath := r.fw.PathFor(a.Type, a.Slug)
	aBefore, err := os.ReadFile(aPath)
	if err != nil {
		t.Fatal(err)
	}

	_, err = Merge(r.q, r.fw, r.ss, a, b, mergeFixedNow, "human:david")
	if !errors.Is(err, ErrShardTierUnsupported) {
		t.Fatalf("err = %v, want ErrShardTierUnsupported", err)
	}

	// Refusal must be a no-op: neither page touched.
	aAfter, _ := os.ReadFile(aPath)
	if !bytes.Equal(aBefore, aAfter) {
		t.Fatal("a's page was modified despite Merge refusing")
	}
	bAfter, _ := os.ReadFile(bPath)
	if !bytes.Equal(bBefore, bAfter) {
		t.Fatal("b's page was modified despite Merge refusing")
	}
}

func TestMergeRefusesTypeMismatch(t *testing.T) {
	r := newTestRig(t)
	a := domain.Entity{Type: "person", Slug: "acme-corp"}
	b := domain.Entity{Type: "company", Slug: "acme-inc"}
	_, err := Merge(r.q, r.fw, r.ss, a, b, mergeFixedNow, "human:david")
	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("err = %v, want ErrTypeMismatch", err)
	}
}

func TestUndoRestoresPriorStateByteIdentically(t *testing.T) {
	r := newTestRig(t)

	a := domain.Entity{Type: "person", Slug: "acme-corp", Aliases: []string{"Acme"}}
	b := domain.Entity{Type: "person", Slug: "acme-corporation"}

	aPage := store.NewEntityPage(a)
	aPage.Claims = []domain.Claim{fenceClaim("c-a1", a.Slug, "works_at", "acme")}
	aPath := r.seedPage(t, aPage)

	bPage := store.NewEntityPage(b)
	bPage.Claims = []domain.Claim{fenceClaim("c-b1", b.Slug, "prefers", "async standups")}
	bPath := r.seedPage(t, bPage)

	aBeforeMerge, err := os.ReadFile(aPath)
	if err != nil {
		t.Fatal(err)
	}
	bBeforeMerge, err := os.ReadFile(bPath)
	if err != nil {
		t.Fatal(err)
	}

	ev, err := Merge(r.q, r.fw, r.ss, a, b, mergeFixedNow, "human:david")
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	// Sanity: the merge actually changed things before we undo it.
	aAfterMerge, _ := os.ReadFile(aPath)
	if bytes.Equal(aBeforeMerge, aAfterMerge) {
		t.Fatal("merge did not actually change a's page -- test fixture is broken")
	}
	if _, err := os.Stat(bPath); !os.IsNotExist(err) {
		t.Fatal("merge did not remove b's page -- test fixture is broken")
	}

	undone, err := Undo(r.q, r.root, ev.ID, mergeFixedNow.Add(time.Hour))
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if undone.UndoneAt == nil {
		t.Fatal("Undo did not stamp UndoneAt")
	}

	aRestored, err := os.ReadFile(aPath)
	if err != nil {
		t.Fatalf("read restored a: %v", err)
	}
	if !bytes.Equal(aRestored, aBeforeMerge) {
		t.Fatalf("a's page not byte-identical after undo:\n--- want ---\n%s\n--- got ---\n%s", aBeforeMerge, aRestored)
	}

	bRestored, err := os.ReadFile(bPath)
	if err != nil {
		t.Fatalf("read restored b: %v", err)
	}
	if !bytes.Equal(bRestored, bBeforeMerge) {
		t.Fatalf("b's page not byte-identical after undo:\n--- want ---\n%s\n--- got ---\n%s", bBeforeMerge, bRestored)
	}
}

func TestUndoRestoresPageThatDidNotExistBeforeMerge(t *testing.T) {
	r := newTestRig(t)

	a := domain.Entity{Type: "person", Slug: "acme-corp"}
	b := domain.Entity{Type: "person", Slug: "acme-corporation", Aliases: []string{"Acme Inc"}}
	aPath := r.seedPage(t, store.NewEntityPage(a))
	// b has no page on disk at all.
	bPath := r.fw.PathFor(b.Type, b.Slug)

	ev, err := Merge(r.q, r.fw, r.ss, a, b, mergeFixedNow, "human:david")
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if ev.BExisted {
		t.Fatal("BExisted = true, want false: b never had a page")
	}

	if _, err := Undo(r.q, r.root, ev.ID, mergeFixedNow.Add(time.Hour)); err != nil {
		t.Fatalf("Undo: %v", err)
	}
	if _, err := os.Stat(bPath); !os.IsNotExist(err) {
		t.Fatalf("b's page must not exist after undoing a merge where b never had one (err=%v)", err)
	}
	if _, err := os.Stat(aPath); err != nil {
		t.Fatalf("a's page must still exist after undo: %v", err)
	}
}

func TestUndoTwiceRefusesSecondCall(t *testing.T) {
	r := newTestRig(t)
	a := domain.Entity{Type: "person", Slug: "acme-corp"}
	b := domain.Entity{Type: "person", Slug: "acme-corporation"}
	r.seedPage(t, store.NewEntityPage(a))
	r.seedPage(t, store.NewEntityPage(b))

	ev, err := Merge(r.q, r.fw, r.ss, a, b, mergeFixedNow, "human:david")
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if _, err := Undo(r.q, r.root, ev.ID, mergeFixedNow.Add(time.Hour)); err != nil {
		t.Fatalf("first Undo: %v", err)
	}
	if _, err := Undo(r.q, r.root, ev.ID, mergeFixedNow.Add(2*time.Hour)); !errors.Is(err, ErrAlreadyUndone) {
		t.Fatalf("second Undo err = %v, want ErrAlreadyUndone", err)
	}
}

func TestUndoUnknownEventErrors(t *testing.T) {
	r := newTestRig(t)
	if _, err := Undo(r.q, r.root, "nonexistent", mergeFixedNow); !errors.Is(err, ErrEventNotFound) {
		t.Fatalf("err = %v, want ErrEventNotFound", err)
	}
}

// TestMergeEventPersistedUnderRuntimeDir pins where MergeEvent lands on
// disk, per its own doc: .serenity/entities/merges/<id>.json.
func TestMergeEventPersistedUnderRuntimeDir(t *testing.T) {
	r := newTestRig(t)
	a := domain.Entity{Type: "person", Slug: "acme-corp"}
	b := domain.Entity{Type: "person", Slug: "acme-corporation"}
	r.seedPage(t, store.NewEntityPage(a))
	r.seedPage(t, store.NewEntityPage(b))

	ev, err := Merge(r.q, r.fw, r.ss, a, b, mergeFixedNow, "human:david")
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	want := filepath.Join(r.root, ".serenity", "entities", "merges", ev.ID+".json")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("merge event not found at %s: %v", want, err)
	}
}

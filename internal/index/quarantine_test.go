package index

import (
	"bytes"
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

// quarantineFixture builds a brain with two well-formed person pages
// (good-a, good-b) and one page whose frontmatter carries a duplicate
// `type:` key -- the SEC-H03 injection shape: the extraction subject
// smuggled a newline plus a second YAML key into the page header. The
// bad page is rendered by the real FenceWriter and then corrupted in
// place, so everything else about it is canonical. Returns the bad
// page's path.
func quarantineFixture(t *testing.T, root string) string {
	t.Helper()
	fw := store.NewFenceWriter(root)
	for _, slug := range []string{"good-a", "good-b"} {
		p := store.NewEntityPage(domain.Entity{Type: "person", Slug: slug})
		p.Summary = "Fixture person " + slug + "."
		p.Claims = []domain.Claim{{
			ID: slug + "-c1", SubjectSlug: slug, Predicate: "works_at", Family: "works_at",
			Object: "Acme", Confidence: 0.9, State: domain.StateActive, SourceRef: "e1#1",
		}}
		if _, err := fw.WriteEntity(p); err != nil {
			t.Fatal(err)
		}
	}
	bad := store.NewEntityPage(domain.Entity{Type: "person", Slug: "injected"})
	bad.Summary = "Injected page."
	raw, err := fw.RenderEntity(bad)
	if err != nil {
		t.Fatal(err)
	}
	head := []byte("---\ntype: person\n")
	if !bytes.HasPrefix(raw, head) {
		t.Fatalf("fixture render did not start with %q:\n%s", head, raw)
	}
	raw = bytes.Replace(raw, head, []byte("---\ntype: person\ntype: person\n"), 1)
	path := fw.PathFor("person", "injected")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func openQuarantineEngine(t *testing.T, root string) *SQLite {
	t.Helper()
	dbPath := filepath.Join(root, ".serenity", "index.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	eng, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	return eng
}

// TestRebuildQuarantinesUnparsablePage is T24.5's fixture: one corrupt
// page must not take the whole brain's index down. Rebuild succeeds and
// indexes exactly the two good pages.
func TestRebuildQuarantinesUnparsablePage(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	quarantineFixture(t, root)
	eng := openQuarantineEngine(t, root)

	if err := Rebuild(ctx, root, config.Default(), eng); err != nil {
		t.Fatalf("Rebuild must skip the unparsable page and continue, got: %v", err)
	}
	stats, err := eng.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats["entities"] != 2 {
		t.Fatalf("indexed entities = %d, want 2 (good-a, good-b)", stats["entities"])
	}
	dump, err := DumpString(ctx, eng)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"good-a", "good-b"} {
		if !bytes.Contains([]byte(dump), []byte(want)) {
			t.Fatalf("dump lacks %s:\n%s", want, dump)
		}
	}
	if bytes.Contains([]byte(dump), []byte("injected")) {
		t.Fatalf("quarantined page leaked into the index:\n%s", dump)
	}
}

// TestAuditPagesReportsQuarantineAndNonConformingSlugs: the audit that
// `serenity check` prints has exactly one quarantine entry for the
// injected page (root-relative path, the yaml duplicate-key error) and
// reports a parsable page whose slug fails domain.ValidSlug -- written by
// hand, since the writer will refuse such a slug once T24.4 lands and a
// pre-existing page is precisely what the audit exists to find.
func TestAuditPagesReportsQuarantineAndNonConformingSlugs(t *testing.T) {
	root := t.TempDir()
	badPath := quarantineFixture(t, root)
	fw := store.NewFenceWriter(root)
	raw, err := fw.RenderEntity(store.NewEntityPage(domain.Entity{Type: "person", Slug: "ok-slug"}))
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.Replace(raw, []byte("slug: ok-slug\n"), []byte("slug: Not_Canonical\n"), 1)
	shoutyPath := fw.PathFor("person", "Not_Canonical")
	if err := os.WriteFile(shoutyPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	audit, err := AuditPages(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(audit.Quarantined) != 1 {
		t.Fatalf("quarantined = %+v, want exactly the injected page", audit.Quarantined)
	}
	if q := audit.Quarantined[0]; q.Path != filepath.Join("brain", "entities", "person", "injected.md") || q.Err == nil || !strings.Contains(q.Err.Error(), `mapping key "type" already defined`) {
		t.Fatalf("quarantine entry = %+v (path %q), want root-relative path and the duplicate-key error", q, badPath)
	}
	if len(audit.NonConformingSlugs) != 1 {
		t.Fatalf("non-conforming = %+v, want exactly Not_Canonical", audit.NonConformingSlugs)
	}
	if n := audit.NonConformingSlugs[0]; n.Slug != "Not_Canonical" || n.Path != filepath.Join("brain", "entities", "person", "Not_Canonical.md") {
		t.Fatalf("non-conforming entry = %+v", n)
	}

	// The same walk backs Rebuild: the quarantine slice it sees has the
	// same single entry, with the absolute path it logs.
	_, quarantined, err := WalkEntityPages(root, fw)
	if err != nil {
		t.Fatal(err)
	}
	if len(quarantined) != 1 || quarantined[0].Path != badPath {
		t.Fatalf("WalkEntityPages quarantined = %+v, want [%s]", quarantined, badPath)
	}
}

// TestRetrievalEligibilityFailsClosedForQuarantinedPage: a stale index row
// for a page that has since become unparsable is refused at read time --
// quarantine narrows what is served, never widens it.
func TestRetrievalEligibilityFailsClosedForQuarantinedPage(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	fw := store.NewFenceWriter(root)
	p := store.NewEntityPage(domain.Entity{Type: "person", Slug: "later-corrupt"})
	p.Summary = "Was fine once."
	if _, err := fw.WriteEntity(p); err != nil {
		t.Fatal(err)
	}
	eng := openQuarantineEngine(t, root)
	if err := Rebuild(ctx, root, config.Default(), eng); err != nil {
		t.Fatal(err)
	}
	hits, err := eng.AllChunks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var pageHit *Hit
	for i := range hits {
		if hits[i].ChunkRef == "page:later-corrupt" {
			pageHit = &hits[i]
		}
	}
	if pageHit == nil {
		t.Fatalf("no page chunk indexed: %+v", hits)
	}

	// Corrupt the page in place after indexing; do not rebuild.
	path := fw.PathFor("person", "later-corrupt")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.Replace(raw, []byte("---\ntype: person\n"), []byte("---\ntype: person\ntype: person\n"), 1)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	proj, err := store.LoadMemoryProjection(store.NewSourceStore(root))
	if err != nil {
		t.Fatal(err)
	}
	eligible, err := RetrievalEligibility(root, proj, false, false, time.Now())
	if err != nil {
		t.Fatalf("eligibility must not fail for the whole brain, got: %v", err)
	}
	if eligible(*pageHit) {
		t.Fatalf("stale chunk for a quarantined page must be refused: %+v", *pageHit)
	}
}

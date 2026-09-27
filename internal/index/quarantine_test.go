package index

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

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

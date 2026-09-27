package compose

import (
	"bytes"
	"os"
	"testing"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
)

// quarantineFixture mirrors internal/index's fixture of the same name:
// two well-formed person pages plus one page whose frontmatter carries a
// duplicate `type:` key (the SEC-H03 injection shape). Returns the bad
// page's path.
func quarantineFixture(t *testing.T, root string) string {
	t.Helper()
	fw := store.NewFenceWriter(root)
	for _, slug := range []string{"good-a", "good-b"} {
		p := store.NewEntityPage(domain.Entity{Type: "person", Slug: slug})
		p.Claims = []domain.Claim{{
			ID: slug + "-c1", SubjectSlug: slug, Predicate: "works_at", Family: "works_at",
			Object: "Acme", Confidence: 0.9, State: domain.StateActive, SourceRef: "e1#1",
		}}
		if _, err := fw.WriteEntity(p); err != nil {
			t.Fatal(err)
		}
	}
	bad := store.NewEntityPage(domain.Entity{Type: "person", Slug: "injected"})
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

// TestAllClaimsQuarantinesUnparsablePage: ask and MCP synthesize walk
// every page through AllClaims, so one corrupt page must not abort
// composition for the whole brain. AllClaims succeeds and returns the
// two good pages' claims.
func TestAllClaimsQuarantinesUnparsablePage(t *testing.T) {
	root := t.TempDir()
	quarantineFixture(t, root)

	bySubject, err := AllClaims(root, config.Default())
	if err != nil {
		t.Fatalf("AllClaims must skip the unparsable page and continue, got: %v", err)
	}
	for _, slug := range []string{"good-a", "good-b"} {
		if n := len(bySubject[slug]); n != 1 {
			t.Fatalf("claims for %s = %d, want 1 (got %+v)", slug, n, bySubject)
		}
	}
	if _, leaked := bySubject["injected"]; leaked {
		t.Fatalf("quarantined page leaked into AllClaims: %+v", bySubject)
	}
}

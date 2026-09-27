package memory

import (
	"bytes"
	"os"
	"testing"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
)

// writeInjectedPage writes one person page under root whose frontmatter
// carries a duplicate `type:` key (the SEC-H03 injection shape) and
// returns its path. It is rendered by the real FenceWriter and corrupted
// in place, so everything else about it is canonical.
func writeInjectedPage(t *testing.T, root string) string {
	t.Helper()
	fw := store.NewFenceWriter(root)
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

// TestEntityToolQuarantinesUnparsablePage: the MCP entity tool walks every
// page on each call, so one corrupt page must not turn every lookup for
// the whole brain into an error. The good pages still resolve.
func TestEntityToolQuarantinesUnparsablePage(t *testing.T) {
	h, root := newTestHandlers(t)
	entityReviewPage(t, h, "person", "good-a", "Good A")
	entityReviewPage(t, h, "person", "good-b", "Good B")
	writeInjectedPage(t, root)

	for _, name := range []string{"person/good-a", "Good B"} {
		got := acceptanceCall(t, h, "entity", map[string]any{"name": name})
		if got["found"] != true {
			t.Fatalf("entity %q: want found=true despite the quarantined page, got %v", name, got)
		}
	}
	got := acceptanceCall(t, h, "entity", map[string]any{"name": "person/injected"})
	if got["found"] != false {
		t.Fatalf("entity person/injected: quarantined page must not resolve, got %v", got)
	}
}

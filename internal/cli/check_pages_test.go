package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
)

// seedPageAuditFixture writes, under an initialized brain root, one
// well-formed person page, one page whose frontmatter carries a duplicate
// `type:` key (the SEC-H03 injection shape, unparsable) and one page whose
// slug fails domain.ValidSlug. The last two are rendered by the real
// FenceWriter for a valid slug and then rewritten in place, so the fixture
// keeps working once the writer itself starts refusing invalid slugs
// (T24.4).
func seedPageAuditFixture(t *testing.T, root string) {
	t.Helper()
	fw := store.NewFenceWriter(root)
	good := store.NewEntityPage(domain.Entity{Type: "person", Slug: "good-a"})
	if _, err := fw.WriteEntity(good); err != nil {
		t.Fatal(err)
	}

	render := func(slug string) []byte {
		t.Helper()
		raw, err := fw.RenderEntity(store.NewEntityPage(domain.Entity{Type: "person", Slug: slug}))
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	write := func(path string, raw []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	head := []byte("---\ntype: person\n")
	injected := render("injected")
	if !bytes.HasPrefix(injected, head) {
		t.Fatalf("fixture render did not start with %q:\n%s", head, injected)
	}
	write(fw.PathFor("person", "injected"), bytes.Replace(injected, head, []byte("---\ntype: person\ntype: person\n"), 1))

	shouty := bytes.Replace(render("bad-slug"), []byte("slug: bad-slug\n"), []byte("slug: Bad_Slug\n"), 1)
	if !bytes.Contains(shouty, []byte("slug: Bad_Slug\n")) {
		t.Fatalf("fixture slug rewrite did not apply:\n%s", shouty)
	}
	write(fw.PathFor("person", "Bad_Slug"), shouty)
}

// TestCheckReportsQuarantinedPagesAndNonConformingSlugs: `serenity check`
// prints a `quarantined pages` section naming each unparsable page with
// its parse error, and a `non-conforming slugs` section naming each page
// whose slug fails domain.ValidSlug, so an operator can rename before
// enforcement on read. Both are warning class (ADR 010): they never move
// the exit code, which still follows the plan verdict alone.
func TestCheckReportsQuarantinedPagesAndNonConformingSlugs(t *testing.T) {
	root := initBrainRepo(t)
	seedPageAuditFixture(t, root)

	var out bytes.Buffer
	err := runCheck(context.Background(), root, "", `[{"action":"start_project","params":{}}]`, false, &out)
	if err != nil {
		t.Fatalf("check must exit 0 for no_applicable_constraints regardless of page warnings, got %v\n%s", err, out.String())
	}
	got := out.String()
	for _, want := range []string{
		"status: no_applicable_constraints",
		"quarantined pages: 1",
		filepath.Join("brain", "entities", "person", "injected.md") + ": entity frontmatter:",
		`mapping key "type" already defined`,
		"non-conforming slugs: 1",
		filepath.Join("brain", "entities", "person", "Bad_Slug.md") + `: slug "Bad_Slug"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("check output lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, root) {
		t.Fatalf("check output must print root-relative paths, not %s:\n%s", root, got)
	}
	if strings.Contains(got, "good-a") {
		t.Fatalf("a conforming, parsable page must not be reported:\n%s", got)
	}
}

// TestCheckReportsCleanBrainCounts: a brain with only conforming, parsable
// pages still prints both sections with a zero count, so an operator can
// tell the audit ran.
func TestCheckReportsCleanBrainCounts(t *testing.T) {
	root := initBrainRepo(t)
	fw := store.NewFenceWriter(root)
	if _, err := fw.WriteEntity(store.NewEntityPage(domain.Entity{Type: "person", Slug: "good-a"})); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runCheck(context.Background(), root, "", `[{"action":"start_project","params":{}}]`, false, &out); err != nil {
		t.Fatalf("check: %v\n%s", err, out.String())
	}
	for _, want := range []string{"quarantined pages: 0\n", "non-conforming slugs: 0\n"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("check output lacks %q:\n%s", want, out.String())
		}
	}
}

package store

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/domain"
)

// TestRenderEntityRefusesInvalidSlugOrType is the writer half of the
// SEC-H03 fix: RenderEntity returns an error, and WriteEntity writes
// nothing, for any entity whose slug or type is not a canonical slug. It
// refuses rather than escapes, so a subject can never become a directory
// name or a YAML key in a page header.
func TestRenderEntityRefusesInvalidSlugOrType(t *testing.T) {
	cases := []struct {
		name string
		typ  string
		slug string
	}{
		{"newline slug injects yaml key", "person", "acme\ntype: person"},
		{"newline slug injects aliases", "person", "acme\naliases: [alice-tan]"},
		{"colon slug", "person", "acme:corp"},
		{"uppercase slug", "person", "Acme"},
		{"dot segment slug", "person", ".."},
		{"slash slug", "person", "people/acme"},
		{"backslash slug", "person", "people\\acme"},
		{"unicode slug", "person", "ünïcode"},
		{"empty slug", "person", ""},
		{"long slug", "person", strings.Repeat("a", 65)},
		{"empty type", "", "acme"},
		{"newline type", "person\nslug: alice-tan", "acme"},
		{"slash type", "people/x", "acme"},
		{"uppercase type", "Person", "acme"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			w := NewFenceWriter(root)
			p := NewEntityPage(domain.Entity{Type: tc.typ, Slug: tc.slug})
			p.Claims = []domain.Claim{{ID: "c1", SubjectSlug: tc.slug, Predicate: "works_at", Family: "works_at", Object: "acme", State: domain.StateActive}}
			if _, err := w.RenderEntity(p); err == nil {
				t.Fatalf("RenderEntity accepted type %q slug %q", tc.typ, tc.slug)
			}
			if _, err := w.WriteEntity(p); err == nil {
				t.Fatalf("WriteEntity accepted type %q slug %q", tc.typ, tc.slug)
			}
			var written []string
			err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if !entry.IsDir() {
					written = append(written, path)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(written) != 0 {
				t.Fatalf("WriteEntity wrote files for a refused entity: %v", written)
			}
		})
	}
}

// TestFrontmatterIsYAMLMarshalled pins that the page header is emitted by
// yaml.Marshal, not by string formatting: an alias containing a colon is
// quoted, a slug that YAML would otherwise read as a number is quoted and
// still parses back as the same string, and the whole page round-trips
// byte-identically.
func TestFrontmatterIsYAMLMarshalled(t *testing.T) {
	w := NewFenceWriter(t.TempDir())

	p := NewEntityPage(domain.Entity{Type: "person", Slug: "ava", Aliases: []string{"name: value", "Ava", "Ava, Example"}})
	raw, err := w.RenderEntity(p)
	if err != nil {
		t.Fatal(err)
	}
	wantHeader := "---\ntype: person\nslug: ava\naliases: ['name: value', Ava, 'Ava, Example']\n---\n"
	if !bytes.HasPrefix(raw, []byte(wantHeader)) {
		t.Fatalf("frontmatter is not yaml.Marshal output:\nwant prefix:\n%s\ngot:\n%s", wantHeader, raw)
	}
	parsed, err := ParseEntityBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Entity.Aliases) != 3 || parsed.Entity.Aliases[0] != "name: value" {
		t.Fatalf("aliases did not round-trip: %+v", parsed.Entity.Aliases)
	}

	numeric := NewEntityPage(domain.Entity{Type: "person", Slug: "123"})
	raw, err = w.RenderEntity(numeric)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(raw, []byte("---\ntype: person\nslug: \"123\"\n---\n")) {
		t.Fatalf("numeric-looking slug not quoted by yaml.Marshal:\n%s", raw)
	}
	parsed, err = ParseEntityBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Entity.Slug != "123" || parsed.Entity.Type != "person" {
		t.Fatalf("quoted slug did not parse back as the same string: %+v", parsed.Entity)
	}
	again, err := w.RenderEntity(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, again) {
		t.Fatalf("round trip changed page:\n%s\n%s", raw, again)
	}
}

package gbrain_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/import/gbrain"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/store"
)

const fixture = "../../../testdata/gbrain-fixture"

func brain(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "fixture@example.invalid"}, {"config", "user.name", "Fixture"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v: %s", err, out)
		}
	}
	return root
}
func TestImportFixture(t *testing.T) {
	root := brain(t)
	ctx := context.Background()
	cfg := config.Default()
	got, err := gbrain.Import(ctx, fixture, root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got.Pages != 2 || got.Claims != 8 || got.Skipped != 0 {
		t.Fatalf("counts: %+v", got)
	}
	fw := store.NewFenceWriter(root)
	p, err := fw.ParseEntity(fw.PathFor("person", "ava"))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Claims) != 7 || len(p.Timeline) != 2 || len(p.Links) != 1 {
		t.Fatalf("missing data: %+v", p)
	}
	if p.Links[0].Target != "projects/beacon" || p.Links[0].Kind != "links_to" || p.Links[0].EntitySlug != "beacon" {
		t.Fatalf("edge: %+v", p.Links)
	}
	if p.Frontmatter["custom_note"] != "preserved verbatim" || !strings.Contains(p.OriginalBody, "Synthetic person") {
		t.Fatal("lost original metadata or prose")
	}
	byKey := map[string]domain.Claim{}
	ids := map[string]bool{}
	for _, c := range p.Claims {
		if !c.Review || c.Confidence > .9 || c.SourceRef != "gbrain:people/ava#"+c.Provenance.Meta["#"] {
			t.Fatalf("mapping: %+v", c)
		}
		if ids[c.ID] {
			t.Fatal("duplicate claim ID")
		}
		ids[c.ID] = true
		byKey[c.Provenance.Meta["gbrain_fence"]+c.Provenance.Meta["#"]] = c
	}
	if byKey["facts1"].SupersededBy != byKey["facts2"].ID || byKey["facts1"].State != domain.StateSuperseded {
		t.Fatal("lost supersession")
	}
	if byKey["facts3"].State != domain.StateRetracted || byKey["facts3"].Provenance.Meta["forgotten_reason"] != "forgotten: user request" {
		t.Fatal("lost retraction")
	}
	if byKey["takes1"].ValidFrom != "2026-02" || byKey["takes1"].ValidTo != "2026-04" || byKey["takes1"].Provenance.Actor != "people/ava" {
		t.Fatal("lost take attribution/window")
	}
	if byKey["facts2"].Visibility != domain.VisibilityPrivate || byKey["facts2"].Provenance.Meta["context"] != "choice | rationale" || byKey["facts4"].Provenance.Meta["context"] != `C:\notes\ava` {
		t.Fatal("lost privacy or escaped cell")
	}
	original, err := os.ReadFile(fw.PathFor("person", "ava"))
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := fw.RenderEntity(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(original) != string(rendered) {
		t.Fatal("parse/render changed canonical bytes")
	}
	again, err := gbrain.Import(ctx, fixture, root, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if again.Skipped != 2 {
		t.Fatalf("repeat: %+v", again)
	}
	eng, err := index.Open(filepath.Join(root, "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()
	if err := index.Rebuild(ctx, root, cfg, eng); err != nil {
		t.Fatal(err)
	}
	first, err := index.DumpString(ctx, eng)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(first, "Prefer feature flags") {
		t.Fatal("imported claims absent from rebuilt index")
	}
	if err := index.Rebuild(ctx, root, cfg, eng); err != nil {
		t.Fatal(err)
	}
	second, err := index.DumpString(ctx, eng)
	if err != nil || first != second {
		t.Fatal("rebuild changed index")
	}
}
func TestMalformedInputIsRejectedBeforeWrites(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(fixture, "people/ava.md"))
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"missing marker":        strings.Replace(string(raw), "<!--- gbrain:facts:end -->", "", 1),
		"duplicate row":         strings.Replace(string(raw), "| 5 | Tests", "| 4 | Tests", 1),
		"unknown visibility":    strings.Replace(string(raw), "| 0.85 | private |", "| 0.85 | public |", 1),
		"invalid score":         strings.Replace(string(raw), "| 0.85 |", "| NaN |", 1),
		"dangling supersession": strings.Replace(string(raw), "superseded by #2", "superseded by #999", 1),
		"self supersession":     strings.Replace(string(raw), "superseded by #2", "superseded by #1", 1),
		"missing column":        strings.Replace(string(raw), "| context |", "| mystery |", 1),
		"unsafe type":           strings.Replace(string(raw), "type: person", "type: ../../escape", 1),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			src := t.TempDir()
			if err := os.WriteFile(filepath.Join(src, "ava.md"), []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			root := brain(t)
			if _, err := gbrain.Import(context.Background(), src, root, config.Default()); err == nil {
				t.Fatal("accepted malformed input")
			}
			if _, err := os.Stat(filepath.Join(root, "brain")); !os.IsNotExist(err) {
				t.Fatal("wrote canonical files before validation")
			}
		})
	}
}
func TestImportPreservesDifferentExistingPage(t *testing.T) {
	root := brain(t)
	fw := store.NewFenceWriter(root)
	path := fw.PathFor("person", "ava")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte("human work, not tracked yet\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := gbrain.Import(context.Background(), fixture, root, config.Default()); err == nil {
		t.Fatal("overwrote human work")
	}
	after, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(after, original) {
		t.Fatal("human bytes changed")
	}
}
func TestImportRejectsSymlinkDestination(t *testing.T) {
	root := brain(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "brain")); err != nil {
		t.Fatal(err)
	}
	if _, err := gbrain.Import(context.Background(), fixture, root, config.Default()); err == nil {
		t.Fatal("accepted escaping destination")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) > 0 {
		t.Fatal("wrote outside brain")
	}
}

func TestTakeSupersessionAndUnicodeRange(t *testing.T) {
	raw := []byte("---\ntype: person\n---\n# Example\n<!--- gbrain:takes:begin -->\n| # | claim | kind | who | weight | since | source |\n|---|---|---|---|---|---|---|\n| 1 | ~~Old prediction~~ | bet | people/ava | 0.12345 | 2025-01 → 2025-06 | superseded by #2 |\n| 2 | New prediction | bet | people/ava | 0.8 | 2025-06 | conversation |\n<!--- gbrain:takes:end -->\n")
	p, err := gbrain.Parse("people/ava.md", raw)
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := gbrain.Map(p)
	if err != nil {
		t.Fatal(err)
	}
	a, b := mapped.Claims[0], mapped.Claims[1]
	if a.State != domain.StateSuperseded || a.SupersededBy != b.ID || a.ValidFrom != "2025-01" || a.ValidTo != "2025-06" || a.Confidence != .12345 {
		t.Fatalf("incorrect take mapping: %+v", a)
	}
}

package writer

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
)

func TestFenceDerivedPreservesOutsideSections(t *testing.T) {
	root := t.TempDir()
	gitRepo(t, root)
	fw := store.NewFenceWriter(root)
	q := NewQueue(nil)
	t.Cleanup(q.Close)
	p := store.NewEntityPage(domain.Entity{Type: "person", Slug: "alice"})
	p.Summary = "old derived summary"
	path, original, err := Fence(q, fw, p)
	if err != nil {
		t.Fatal(err)
	}
	original = bytes.Replace(original, []byte("slug: alice\n"), []byte("slug: alice\ncustom: preserve-me\n"), 1)
	original = bytes.Replace(original, []byte("# Alice\n"), []byte("# Alice\n\nA hand-written introduction.\n"), 1)
	original = append(original, []byte("\n## Personal notes\nKeep this exact prose.\n")...)
	handEdit(t, path, original)
	gitCommitAll(t, root, "human notes")
	p.Summary = "fresh derived summary"
	_, got, err := FenceDerived(q, fw, p)
	if err != nil {
		t.Fatal(err)
	}
	want := bytes.Replace(original, []byte("old derived summary"), []byte("fresh derived summary"), 1)
	if !bytes.Equal(got, want) {
		t.Fatalf("outside sections changed:\n%s", got)
	}
	// An unchanged rerun must succeed despite its own previous uncommitted write.
	_, again, err := FenceDerived(q, fw, p)
	if err != nil || !bytes.Equal(again, got) {
		t.Fatalf("repeat: %v", err)
	}
}

func TestFenceDerivedProtectsDirtyEdits(t *testing.T) {
	root := t.TempDir()
	gitRepo(t, root)
	fw := store.NewFenceWriter(root)
	q := NewQueue(nil)
	t.Cleanup(q.Close)
	p := store.NewEntityPage(domain.Entity{Type: "person", Slug: "alice"})
	p.Summary = "baseline"
	path, original, err := Fence(q, fw, p)
	if err != nil {
		t.Fatal(err)
	}
	gitCommitAll(t, root, "baseline")
	human := append(original, []byte("\nUncommitted human note.\n")...)
	handEdit(t, path, human)
	p.Summary = "machine summary"
	_, _, err = FenceDerived(q, fw, p)
	if !errors.Is(err, ErrDirtyTree) {
		t.Fatalf("got %v, want dirty error", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, human) {
		t.Fatalf("dirty edit changed: %v", err)
	}
	if _, err := os.Stat(PendingPath(root, "alice")); err != nil {
		t.Fatal(err)
	}
}

func TestFenceDerivedDetailAndMalformedSections(t *testing.T) {
	root := t.TempDir()
	fw := store.NewFenceWriter(root)
	q := NewQueue(nil)
	t.Cleanup(q.Close)
	p := store.NewEntityPage(domain.Entity{Type: "person", Slug: "alice"})
	path, _, err := Fence(q, fw, p)
	if err != nil {
		t.Fatal(err)
	}
	p.Claims = []domain.Claim{{ID: "c1", SubjectSlug: "alice", Predicate: "works_at", Object: strings.Repeat("x", 150), State: domain.StateActive}}
	if _, _, err := FenceDerived(q, fw, p); err != nil {
		t.Fatal(err)
	}
	parsed, err := fw.ParseEntity(path)
	if err != nil || len(parsed.Claims) != 1 || parsed.Claims[0].Object != p.Claims[0].Object {
		t.Fatalf("detail roundtrip: %+v, %v", parsed, err)
	}
	p.Claims = nil
	_, got, err := FenceDerived(q, fw, p)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(got, []byte("claims-detail:begin")) {
		t.Fatal("obsolete detail retained")
	}
	bad := bytes.Replace(got, []byte("<!-- serenity:summary:end -->"), nil, 1)
	handEdit(t, path, bad)
	if _, _, err := FenceDerived(q, fw, p); err == nil {
		t.Fatal("malformed section accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, bad) {
		t.Fatalf("malformed page changed: %v", err)
	}
}

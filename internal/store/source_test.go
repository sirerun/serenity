package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/domain"
)

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// TestSourceStoreContentAddressedLayout asserts the RFC §7.4 on-disk
// layout: brain/sources/<sha256[0:2]>/<sha256>/ holding raw bytes plus a
// meta.yaml sidecar, and that Write derives SHA256 from the bytes rather
// than trusting the caller.
func TestSourceStoreContentAddressedLayout(t *testing.T) {
	root := t.TempDir()
	s := NewSourceStore(root)
	data := []byte("hello world, this is a raw source body")
	src := domain.Source{
		Kind:       "file",
		URI:        "file:///tmp/a.txt",
		OccurredAt: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC),
	}

	got, err := s.Write(data, src)
	if err != nil {
		t.Fatal(err)
	}
	wantSHA := sha256Hex(data)
	if got.SHA256 != wantSHA {
		t.Fatalf("sha256 = %s, want %s", got.SHA256, wantSHA)
	}

	dir := filepath.Join(root, "brain", "sources", wantSHA[:2], wantSHA)
	raw, err := os.ReadFile(filepath.Join(dir, "bytes"))
	if err != nil {
		t.Fatalf("bytes file not written under content-addressed dir: %v", err)
	}
	if string(raw) != string(data) {
		t.Fatalf("bytes file content mismatch: got %q want %q", raw, data)
	}

	metaBytes, err := os.ReadFile(filepath.Join(dir, "meta.yaml"))
	if err != nil {
		t.Fatalf("meta.yaml sidecar not written: %v", err)
	}
	meta := string(metaBytes)
	for _, want := range []string{"kind: file", "uri: file:///tmp/a.txt", "2026-08-01T12:00:00Z"} {
		if !strings.Contains(meta, want) {
			t.Fatalf("meta.yaml missing %q:\n%s", want, meta)
		}
	}
}

// TestSourceStoreDedupOnRawBytes asserts identity is derived from the raw
// bytes alone: two writes of the same content under different logical
// metadata resolve to the same sha and the same on-disk directory, and the
// second write does not clobber the first write's metadata (Source is
// immutable, §7.6).
func TestSourceStoreDedupOnRawBytes(t *testing.T) {
	root := t.TempDir()
	s := NewSourceStore(root)
	data := []byte("duplicate raw bytes seen through two different imports")

	first, err := s.Write(data, domain.Source{
		Kind: "email", URI: "imap://first", OccurredAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Write(data, domain.Source{
		Kind: "email", URI: "imap://second", OccurredAt: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}

	if first.SHA256 != second.SHA256 {
		t.Fatalf("identical bytes must dedup onto one sha: %s != %s", first.SHA256, second.SHA256)
	}
	if second.URI != "imap://first" {
		t.Fatalf("second write must not clobber the first write's metadata: got uri %q", second.URI)
	}

	entries, err := os.ReadDir(filepath.Join(root, "brain", "sources", first.SHA256[:2]))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("dedup on raw bytes should produce exactly one content-addressed directory, got %d", len(entries))
	}
}

// TestSourceStoreWriteDedupNoOp asserts that writing identical bytes and
// metadata a second time is a true filesystem no-op — the raw file is
// never rewritten, which is what makes Source immutability more than a
// documentation claim.
func TestSourceStoreWriteDedupNoOp(t *testing.T) {
	root := t.TempDir()
	s := NewSourceStore(root)
	data := []byte("idempotent write body")
	src := domain.Source{Kind: "file", URI: "file:///tmp/idempotent.txt", OccurredAt: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)}

	first, err := s.Write(data, src)
	if err != nil {
		t.Fatal(err)
	}

	bytesPath := s.bytesPath(first.SHA256)
	before, err := os.Stat(bytesPath)
	if err != nil {
		t.Fatal(err)
	}
	// Force the mtime backward so a real rewrite would be observable.
	past := before.ModTime().Add(-1 * time.Hour).Truncate(time.Second)
	if err := os.Chtimes(bytesPath, past, past); err != nil {
		t.Fatal(err)
	}

	if _, err := s.Write(data, src); err != nil {
		t.Fatal(err)
	}

	after, err := os.Stat(bytesPath)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(past) {
		t.Fatalf("second identical write touched the bytes file on disk: mtime moved from %v to %v", past, after.ModTime())
	}
}

// TestSourceStoreIndexOnlyGitignore asserts that an index_only source's raw
// bytes are excluded from git and its meta.yaml records index_only: true
// (RFC §7.4: large or sensitive originals stay on disk, out of git).
func TestSourceStoreIndexOnlyGitignore(t *testing.T) {
	root := t.TempDir()
	s := NewSourceStore(root)
	data := []byte("sensitive raw bytes that must never enter git")

	got, err := s.Write(data, domain.Source{
		Kind: "file", URI: "file:///tmp/secret.txt",
		OccurredAt: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
		IndexOnly:  true,
	})
	if err != nil {
		t.Fatal(err)
	}

	gi, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatalf("index_only write must add a .gitignore entry: %v", err)
	}
	bytesRel := filepath.ToSlash(filepath.Join("brain", "sources", got.SHA256[:2], got.SHA256, "bytes"))
	if !strings.Contains(string(gi), bytesRel) {
		t.Fatalf(".gitignore missing entry for index_only bytes file %q:\n%s", bytesRel, gi)
	}

	metaBytes, err := os.ReadFile(filepath.Join(root, "brain", "sources", got.SHA256[:2], got.SHA256, "meta.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(metaBytes), "index_only: true") {
		t.Fatalf("meta.yaml must record index_only: true:\n%s", metaBytes)
	}
}

// TestSourceStoreNonIndexOnlyNoGitignore asserts the converse: an ordinary
// (non-index_only) source is never gitignored.
func TestSourceStoreNonIndexOnlyNoGitignore(t *testing.T) {
	root := t.TempDir()
	s := NewSourceStore(root)
	data := []byte("ordinary raw bytes, fine to commit")

	if _, err := s.Write(data, domain.Source{
		Kind: "file", URI: "file:///tmp/ok.txt", OccurredAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(root, ".gitignore")); !os.IsNotExist(err) {
		t.Fatalf("a non-index_only write must not create a .gitignore entry, stat err = %v", err)
	}
}

// TestTombstoneEmptyForUncitedSha asserts Tombstone returns nothing when no
// claim's provenance cites the given sha.
func TestTombstoneEmptyForUncitedSha(t *testing.T) {
	root := t.TempDir()
	ss := NewShardStore(root)
	if err := ss.Append(domain.Claim{
		SubjectSlug: "alice-tan", Predicate: "has_balance", Family: "has_balance",
		Object: "500.00 usd", Confidence: 0.9, State: domain.StateActive,
		Provenance: domain.Provenance{SourceSHA256: "aaaa1111", ObservedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
	}); err != nil {
		t.Fatal(err)
	}

	s := NewSourceStore(root)
	got, err := s.Tombstone("bbbb2222-never-cited", ss)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no citing claims for an uncited sha, got %d: %+v", len(got), got)
	}
}

// TestTombstoneFindsCitingClaims asserts Tombstone returns every claim,
// across entities and families, whose provenance cites the given sha — and
// only those.
func TestTombstoneFindsCitingClaims(t *testing.T) {
	root := t.TempDir()
	ss := NewShardStore(root)
	const sha = "c0ffee00"

	claims := []domain.Claim{
		{
			SubjectSlug: "alice-tan", Predicate: "has_balance", Family: "has_balance",
			Object: "500.00 usd", Confidence: 0.9, State: domain.StateActive,
			Provenance: domain.Provenance{SourceSHA256: sha, ObservedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
		{
			SubjectSlug: "bob-lee", Predicate: "costs", Family: "costs",
			Object: "12.00 usd", Confidence: 0.8, State: domain.StateActive,
			Provenance: domain.Provenance{SourceSHA256: sha, ObservedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
		},
		{
			SubjectSlug: "alice-tan", Predicate: "has_balance", Family: "has_balance",
			Object: "600.00 usd", ObjectKey: "other", Confidence: 0.9, State: domain.StateActive,
			Provenance: domain.Provenance{SourceSHA256: "different-sha", ObservedAt: time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)},
		},
	}
	for _, c := range claims {
		if err := ss.Append(c); err != nil {
			t.Fatal(err)
		}
	}

	s := NewSourceStore(root)
	got, err := s.Tombstone(sha, ss)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 citing claims, got %d: %+v", len(got), got)
	}
	for _, c := range got {
		if c.Provenance.SourceSHA256 != sha {
			t.Fatalf("Tombstone returned a claim not citing %s: %+v", sha, c)
		}
	}
}

// TestSourceStoreExists pins the "is this genuinely new" check T1.15's
// `serenity sync` uses to scope its git commit to sources it actually just
// wrote: false before Write, true after, for the same sha derived from the
// same bytes -- and unaffected by unrelated shas already in the store.
func TestSourceStoreExists(t *testing.T) {
	root := t.TempDir()
	s := NewSourceStore(root)
	data := []byte("exists-check body")
	sha := sha256Hex(data)

	if s.Exists(sha) {
		t.Fatal("Exists reported true before Write")
	}
	if _, err := s.Write(data, domain.Source{Kind: "file"}); err != nil {
		t.Fatal(err)
	}
	if !s.Exists(sha) {
		t.Fatal("Exists reported false after Write")
	}
	if s.Exists(sha256Hex([]byte("a completely different body"))) {
		t.Fatal("Exists reported true for a sha that was never written")
	}
}

// TestSourceStoreAll pins the full-store enumeration primitive `serenity
// extract` uses to run over every source ever ingested: every written
// source comes back exactly once, sorted by SHA256, with its recorded
// metadata intact.
func TestSourceStoreAll(t *testing.T) {
	root := t.TempDir()
	s := NewSourceStore(root)

	if all, err := s.All(); err != nil || len(all) != 0 {
		t.Fatalf("All() on an empty store = %+v, %v, want empty, nil", all, err)
	}

	written := make([]domain.Source, 0, 3)
	kinds := []string{"file", "git_repo", "email"}
	for i, body := range []string{"first source body", "second source body", "third source body"} {
		src, err := s.Write([]byte(body), domain.Source{Kind: kinds[i], URI: fmt.Sprintf("file:///x%d", i)})
		if err != nil {
			t.Fatal(err)
		}
		written = append(written, src)
	}

	all, err := s.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != len(written) {
		t.Fatalf("All() returned %d sources, want %d: %+v", len(all), len(written), all)
	}
	shas := make([]string, len(all))
	for i, s := range all {
		shas[i] = s.SHA256
	}
	if !sort.StringsAreSorted(shas) {
		t.Fatalf("All() is not sorted by SHA256: %+v", shas)
	}

	byKind := map[string]domain.Source{}
	for _, src := range all {
		byKind[src.Kind] = src
	}
	for _, w := range written {
		got, ok := byKind[w.Kind]
		if !ok {
			t.Fatalf("All() missing a source of kind %q", w.Kind)
		}
		if got.SHA256 != w.SHA256 || got.URI != w.URI {
			t.Fatalf("All() returned %+v for kind %q, want SHA256=%q URI=%q", got, w.Kind, w.SHA256, w.URI)
		}
	}
}

package file_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/connector/file"
	"github.com/sirerun/serenity/internal/store"
)

// TestSourceRecordCarriesNoAbsolutePath is T24.24's acc line for the file
// connector (PRIV-03): the committed meta.yaml of a file source names the
// file relative to the watched root plus a path_hash, and never the
// absolute filesystem path, so a pushed brain does not reveal the user's
// directory layout.
func TestSourceRecordCarriesNoAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	abs := writeFixture(t, dir, "notes/plan.txt", "private plan", time.Hour)

	c := file.NewPoll(dir)
	items, _ := pollAll(t, c, nil)
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	src, err := c.ToSource(items[0])
	if err != nil {
		t.Fatalf("ToSource: %v", err)
	}
	if src.URI != "file:notes/plan.txt" {
		t.Fatalf("URI = %q, want %q", src.URI, "file:notes/plan.txt")
	}
	sum := sha256.Sum256([]byte(filepath.ToSlash(filepath.Clean(abs))))
	if got, want := src.Meta["path_hash"], hex.EncodeToString(sum[:]); got != want {
		t.Fatalf("meta path_hash = %q, want %q", got, want)
	}

	brain := t.TempDir()
	written, err := store.NewSourceStore(brain).Write(items[0].Bytes, src)
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	assertNoAbsolutePath(t, brain, written.SHA256, dir)
}

// TestResyncDedupesOnSamePathHash is T24.24's "dedupe on re-sync still
// works across the change" line: a fresh connector (no cursor, as after a
// lost index) re-polling the same tree yields the same uri and path_hash,
// and the content-addressed store keeps exactly one record.
func TestResyncDedupesOnSamePathHash(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.txt", "alpha", time.Hour)
	brain := t.TempDir()
	ss := store.NewSourceStore(brain)

	var uris, hashes, shas []string
	for range 2 {
		c := file.NewPoll(dir)
		items, _ := pollAll(t, c, nil)
		if len(items) != 1 {
			t.Fatalf("got %d items, want 1", len(items))
		}
		src, err := c.ToSource(items[0])
		if err != nil {
			t.Fatalf("ToSource: %v", err)
		}
		w, err := ss.Write(items[0].Bytes, src)
		if err != nil {
			t.Fatalf("Write: %v", err)
		}
		uris = append(uris, w.URI)
		hashes = append(hashes, w.Meta["path_hash"])
		shas = append(shas, w.SHA256)
	}
	if uris[0] != uris[1] || hashes[0] != hashes[1] || shas[0] != shas[1] {
		t.Fatalf("re-sync changed identity: uris %v hashes %v shas %v", uris, hashes, shas)
	}
	if hashes[0] == "" {
		t.Fatal("path_hash missing")
	}
	entries, err := filepath.Glob(filepath.Join(brain, "brain", "sources", "*", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d source records after re-sync, want 1: %v", len(entries), entries)
	}
}

// assertNoAbsolutePath reads the committed meta.yaml for sha and fails if
// it names root (the absolute directory the source came from) or any
// home-directory prefix.
func assertNoAbsolutePath(t *testing.T, brain, sha, root string) {
	t.Helper()
	meta, err := os.ReadFile(filepath.Join(store.NewSourceStore(brain).DirFor(sha), "meta.yaml"))
	if err != nil {
		t.Fatalf("read meta.yaml: %v", err)
	}
	for _, bad := range []string{filepath.ToSlash(root), "/Users/", "/home/"} {
		if strings.Contains(string(meta), bad) {
			t.Fatalf("meta.yaml carries absolute path fragment %q:\n%s", bad, meta)
		}
	}
}

package gitrepo_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/connector/gitrepo"
	"github.com/sirerun/serenity/internal/store"
)

// TestSourceRecordCarriesNoAbsolutePath is T24.24's acc line for the
// git_repo connector (PRIV-03): every committed meta.yaml names the file
// relative to the crawled repository plus a path_hash, and never the
// repository's absolute location on disk.
func TestSourceRecordCarriesNoAbsolutePath(t *testing.T) {
	repo := initRepo(t)
	writeFile(t, repo, "README.md", "# Project\n")
	writeFile(t, repo, "docs/guide.md", "# Guide\n")
	commitAll(t, repo, "seed")
	toplevel := strings.TrimSpace(runGit(t, repo, "rev-parse", "--show-toplevel"))

	var hashes []string
	for range 2 { // a second, cursorless crawl must reproduce the same identifiers
		c := gitrepo.New(gitrepo.Config{RepoRoot: repo})
		items, _, err := c.Poll(context.Background(), nil)
		if err != nil {
			t.Fatalf("Poll: %v", err)
		}
		if len(items) != 2 {
			t.Fatalf("got %d items, want 2", len(items))
		}
		brain := t.TempDir()
		ss := store.NewSourceStore(brain)
		for _, it := range items {
			src, err := c.ToSource(it)
			if err != nil {
				t.Fatalf("ToSource: %v", err)
			}
			h := src.Meta["path_hash"]
			if len(h) != 64 {
				t.Fatalf("%s: path_hash = %q, want a 64-hex sha256", it.Meta["path"], h)
			}
			hashes = append(hashes, h)
			w, err := ss.Write(it.Bytes, src)
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			meta, err := os.ReadFile(filepath.Join(ss.DirFor(w.SHA256), "meta.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			for _, bad := range []string{filepath.ToSlash(repo), filepath.ToSlash(toplevel), "/Users/", "/home/"} {
				if strings.Contains(string(meta), bad) {
					t.Fatalf("meta.yaml carries absolute path fragment %q:\n%s", bad, meta)
				}
			}
		}
	}
	if hashes[0] == hashes[1] {
		t.Fatalf("distinct files share path_hash %s", hashes[0])
	}
	if hashes[0] != hashes[2] || hashes[1] != hashes[3] {
		t.Fatalf("path_hash not stable across crawls: %v", hashes)
	}
}

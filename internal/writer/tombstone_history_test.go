package writer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
)

func TestSourceTombstonePurgesHistoryAndRetryIsIdempotent(t *testing.T) {
	root, git := gitRepoFixture(t)
	sources := store.NewSourceStore(root)
	const body = "tombstone history secret 83f1a2"
	src, err := sources.Write([]byte(body), domain.Source{Kind: "email", URI: "mail://history/83f1a2", OccurredAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	dir := sources.DirFor(src.SHA256)
	relDir, err := filepath.Rel(root, dir)
	if err != nil {
		t.Fatal(err)
	}
	relPath := filepath.ToSlash(relDir)
	git("add", relPath)
	git("commit", "--quiet", "-m", "fixture source")
	oldCommit := strings.TrimSpace(git("rev-parse", "HEAD"))
	oldBlob := strings.TrimSpace(git("rev-parse", "HEAD:"+relPath+"/bytes"))

	if err := os.WriteFile(filepath.Join(root, "retained-history.txt"), []byte("keep this unrelated history\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", "retained-history.txt")
	git("commit", "--quiet", "-m", "unrelated history")
	retainedBlob := strings.TrimSpace(git("rev-parse", "HEAD:retained-history.txt"))
	if !gitObjectExists(t, root, oldBlob) {
		t.Fatal("fixture source blob is not present before tombstone")
	}

	q := NewQueue(nil)
	defer q.Close()
	w := SourceTombstone{Queue: q, Sources: sources, Shards: store.NewShardStore(root)}
	if _, err := w.Tombstone(src.SHA256, time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}

	history := strings.TrimSpace(git("rev-list", "--all", "--", relPath))
	reflog := git("reflog", "--all", "--format=%H")
	blobPresent := gitObjectExists(t, root, oldBlob)
	if history != "" || strings.Contains(reflog, oldCommit) || blobPresent {
		t.Fatalf("source history survived tombstone: refs=%q old_commit_in_reflog=%v old_blob_present=%v (blob=%s)", history, strings.Contains(reflog, oldCommit), blobPresent, oldBlob)
	}

	if strings.TrimSpace(git("rev-list", "--all", "--", "retained-history.txt")) == "" || !strings.Contains(git("log", "--format=%s", "--all", "--", "retained-history.txt"), "unrelated history") {
		t.Fatal("unrelated history was lost during source tombstone")
	}
	if got := strings.TrimSpace(git("show", "HEAD:retained-history.txt")); got != "keep this unrelated history" {
		t.Fatalf("unrelated current file changed: %q", got)
	}
	if !gitObjectExists(t, root, retainedBlob) {
		t.Fatal("unrelated history blob was pruned")
	}

	if _, err := Flush(q, root); err != nil {
		t.Fatal(err)
	}
	newHead := strings.TrimSpace(git("rev-parse", "HEAD"))
	if _, err := w.Tombstone(src.SHA256, time.Date(2026, 9, 30, 0, 1, 0, 0, time.UTC)); err != nil {
		t.Fatalf("idempotent tombstone retry: %v", err)
	}
	if committed, err := Flush(q, root); err != nil || committed {
		t.Fatalf("retry flush: committed=%v err=%v", committed, err)
	}
	if got := strings.TrimSpace(git("rev-parse", "HEAD")); got != newHead {
		t.Fatalf("idempotent retry changed HEAD: %s -> %s", newHead, got)
	}
	if history := strings.TrimSpace(git("rev-list", "--all", "--", relPath)); history != "" {
		t.Fatalf("retry restored source history: %s", history)
	}
}

func gitObjectExists(t *testing.T, root, object string) bool {
	t.Helper()
	cmd := exec.Command("git", "cat-file", "-e", object)
	cmd.Dir = root
	return cmd.Run() == nil
}

package writer

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/store"
)

func TestForgetRemovesSourceFromHistory(t *testing.T) {
	root, git := gitRepoFixture(t)
	q := NewQueue(nil)
	defer q.Close()
	sources := store.NewSourceStore(root)
	w := MemoryFact{Queue: q, Sources: sources}
	now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	rec, err := w.Remember(RememberInput{Fact: "forget history red", Provenance: "test", Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityWorld}, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Flush(q, root); err != nil {
		t.Fatal(err)
	}
	path := filepath.ToSlash(filepath.Join("brain", "sources", rec.Record.SHA256[:2], rec.Record.SHA256))
	oldCommit := strings.TrimSpace(git("rev-parse", "HEAD"))
	oldBlob := strings.TrimSpace(git("rev-parse", "HEAD:"+path+"/bytes"))
	if _, err := w.Forget(rec.Record.SHA256, "requested", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if history := strings.TrimSpace(git("rev-list", "--all", "--", path)); history != "" {
		t.Fatalf("forgotten source still appears in Git history: %s", history)
	}
	if reflog := git("reflog", "--all", "--format=%H"); strings.Contains(reflog, oldCommit) {
		t.Fatalf("forgotten commit remains in reflogs: %s", reflog)
	}
	cmd := exec.Command("git", "cat-file", "-e", oldBlob)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("forgotten fact blob remains after prune: %s", out)
	}
	newHead := strings.TrimSpace(git("rev-parse", "HEAD"))
	marker := strings.TrimSpace(git("rev-parse", "--git-path", "serenity-history-rewrite-push"))
	if !filepath.IsAbs(marker) {
		marker = filepath.Join(root, marker)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("rewrite push marker missing: %v", err)
	}
	if _, err := w.Forget(rec.Record.SHA256, "retry", now.Add(2*time.Minute)); err != nil {
		t.Fatalf("idempotent forget retry: %v", err)
	}
	if got := strings.TrimSpace(git("rev-parse", "HEAD")); got != newHead {
		t.Fatalf("idempotent retry rewrote history again: %s -> %s", newHead, got)
	}
	segment := "brain/claims/person/has_fact.1.jsonl"
	segmentPath := filepath.Join(root, filepath.FromSlash(segment))
	if err := os.MkdirAll(filepath.Dir(segmentPath), 0o700); err != nil {
		t.Fatal(err)
	}
	before := []byte("{}\n")
	if err := os.WriteFile(segmentPath, before, 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", segment)
	git("commit", "--quiet", "-m", "compaction fixture")
	if err := PublishCompactionFiles(q, root, []FileChange{{Path: segment, Before: before}}); err != nil {
		t.Fatalf("compaction after forget: %v", err)
	}
	if committed, err := Flush(q, root); err != nil || !committed {
		t.Fatalf("flush compaction after forget: committed=%v err=%v", committed, err)
	}
	if _, err := os.Stat(segmentPath); !os.IsNotExist(err) {
		t.Fatalf("compaction segment survived purge regression: %v", err)
	}
}

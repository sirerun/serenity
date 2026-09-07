package writer

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/store"
)

func TestMemoryReviewCommitAcceptsBrainRelativePath(t *testing.T) {
	root, _ := gitRepoFixture(t)
	if err := os.WriteFile(filepath.Join(root, "relative.txt"), []byte("human edit"), 0600); err != nil {
		t.Fatal(err)
	}
	committed, err := CommitPath(root, "relative.txt", "review relative path")
	if err != nil || !committed {
		t.Fatalf("brain-relative path was not committed: committed=%v error=%v", committed, err)
	}
}

func TestMemoryReviewFlushRelativeBrainRoot(t *testing.T) {
	root, _ := gitRepoFixture(t)
	t.Chdir(filepath.Dir(root))
	relativeRoot := filepath.Base(root)
	q := NewQueue(nil)
	defer q.Close()
	w := MemoryFact{Queue: q, Sources: store.NewSourceStore(relativeRoot)}
	_, err := w.Remember(RememberInput{Fact: "relative root fact", Provenance: "review fixture", Kind: store.MemoryFactKindFact, Visibility: store.MemoryVisibilityWorld}, time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	committed, err := Flush(q, relativeRoot)
	if err != nil {
		t.Fatalf("relative brain root must flush the files it wrote: %v", err)
	}
	if !committed {
		t.Fatal("relative brain root wrote a fact but did not commit it")
	}
}

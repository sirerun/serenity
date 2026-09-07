package store_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
)

func TestSourceReviewIndexOnlyRejectsGitignoreSymlink(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "human-config")
	const original = "outside configuration\n"
	if err := os.WriteFile(outside, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	_, writeErr := store.NewSourceStore(root).Write([]byte("local source"), domain.Source{Kind: "document", IndexOnly: true, OccurredAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)})
	after, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != original {
		t.Errorf("source write changed an outside file through .gitignore symlink: %q", after)
	}
	if writeErr == nil {
		t.Error("source write accepted a .gitignore symlink instead of rejecting the unsafe path")
	}
}

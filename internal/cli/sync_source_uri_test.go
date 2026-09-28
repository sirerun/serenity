package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/connector"
	"github.com/sirerun/serenity/internal/index"
)

// TestSyncCommitsNoAbsoluteSourcePath is T24.24 end to end (PRIV-03): a
// real `sync` poll of a file connector commits source meta that names no
// absolute path, while the local index maps the committed path_hash back
// to the file.
func TestSyncCommitsNoAbsoluteSourcePath(t *testing.T) {
	root := initBrainRepo(t)
	configureGitIdentity(t, root)

	watched := t.TempDir()
	abs := filepath.Join(watched, "notes", "plan.txt")
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte("private plan"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(abs, old, old); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Connectors.File = &config.FileConnector{Path: watched}
	cfg.Connectors.Roots = []string{watched} // T24.8 path containment: the temp dir is not under home
	if err := os.MkdirAll(filepath.Join(root, ".serenity"), 0o700); err != nil {
		t.Fatal(err)
	}
	eng, err := index.Open(filepath.Join(root, ".serenity", "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Close() })

	var out bytes.Buffer
	if err := pollConnectors(context.Background(), root, cfg, eng, &out); err != nil {
		t.Fatalf("pollConnectors: %v\n%s", err, out.String())
	}

	cmd := exec.Command("git", "log", "-p", "--all")
	cmd.Dir = root
	history, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v\n%s", err, history)
	}
	if !strings.Contains(string(history), "uri: file:notes/plan.txt") {
		t.Fatalf("committed history lacks the root-relative uri:\n%s", history)
	}
	for _, bad := range []string{filepath.ToSlash(watched), "/Users/", "/home/"} {
		if strings.Contains(string(history), bad) {
			t.Fatalf("committed history carries absolute path fragment %q", bad)
		}
	}

	got, ok, err := eng.SourcePath(context.Background(), connector.PathHash(abs))
	if err != nil || !ok || got != abs {
		t.Fatalf("local index SourcePath = %q, %v, %v; want %q", got, ok, err, abs)
	}
}

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
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/secrets"
	"github.com/sirerun/serenity/internal/store"
)

func TestMain(m *testing.M) {
	secrets.MockForTesting() // never touch the real OS keychain from tests
	os.Exit(m.Run())
}

func TestInitScaffoldsBrainRepo(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	var out bytes.Buffer
	if err := runInit(root, &out); err != nil {
		t.Fatal(err)
	}

	for _, d := range []string{"brain/entities", "brain/sources", "brain/claims", ".dira/entries"} {
		if fi, err := os.Stat(filepath.Join(root, d)); err != nil || !fi.IsDir() {
			t.Fatalf("missing directory %s: %v", d, err)
		}
	}
	if _, err := config.Load(filepath.Join(root, config.FileName)); err != nil {
		t.Fatalf("serenity.yml not created/parsable: %v", err)
	}
	gi, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil || !strings.Contains(string(gi), ".serenity/") {
		t.Fatalf(".gitignore missing .serenity/ entry: %v %q", err, gi)
	}
	if !isGitRepo(root) {
		t.Fatal("git repo not initialized")
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "hooks", "post-commit")); err != nil {
		t.Fatalf("post-commit durability hook not installed: %v", err)
	}
	if !strings.Contains(out.String(), "WARNING: no git remote") {
		t.Fatalf("expected loud no-remote warning, got:\n%s", out.String())
	}
	if _, err := secrets.DaemonToken(); err != nil {
		t.Fatalf("daemon auth token not stored: %v", err)
	}
}

func TestInitIdempotent(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	var out bytes.Buffer
	if err := runInit(root, &out); err != nil {
		t.Fatal(err)
	}
	// Hand-edit the config; a second init must not clobber it.
	cfgPath := filepath.Join(root, config.FileName)
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Models.Extraction = "acme/extract-1@2026-08"
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatal(err)
	}

	if err := runInit(root, &out); err != nil {
		t.Fatalf("second init: %v", err)
	}
	cfg2, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg2.Models.Extraction != "acme/extract-1@2026-08" {
		t.Fatalf("second init clobbered config: %+v", cfg2.Models)
	}
}

// TestSyncWipeRebuildViaCLI drives the M0 acceptance criterion through
// the CLI path: sync, wipe .serenity/, sync again — identical dumps.
func TestSyncWipeRebuildViaCLI(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	root := t.TempDir()
	var out bytes.Buffer
	if err := runInit(root, &out); err != nil {
		t.Fatal(err)
	}

	fw := store.NewFenceWriter(root)
	p := store.NewEntityPage(domain.Entity{Type: "person", Slug: "alice-tan"})
	p.Summary = "Runs engineering at Acme."
	p.Claims = []domain.Claim{{
		ID: "c7f3a000", SubjectSlug: "alice-tan", Predicate: "works_at", Family: "works_at",
		Object: "acme", Confidence: 0.92, ValidFrom: "2025-06",
		SourceRef: "e42#3", State: domain.StateActive,
	}}
	if _, err := fw.WriteEntity(p); err != nil {
		t.Fatal(err)
	}
	ss := store.NewShardStore(root)
	if err := ss.Append(domain.Claim{
		SubjectSlug: "alice-tan", Predicate: "has_balance", Family: "has_balance",
		Object: "1200.00 usd", ObjectKey: "acct-1", Confidence: 0.9, State: domain.StateActive,
		Provenance: domain.Provenance{ObservedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)},
	}); err != nil {
		t.Fatal(err)
	}

	if err := runSync(ctx, root, &out); err != nil {
		t.Fatal(err)
	}
	dump1 := dumpIndex(t, root)

	if err := os.RemoveAll(filepath.Join(root, ".serenity")); err != nil {
		t.Fatal(err)
	}
	if err := runSync(ctx, root, &out); err != nil {
		t.Fatal(err)
	}
	dump2 := dumpIndex(t, root)

	if dump1 != dump2 || dump1 == "" {
		t.Fatalf("wipe-and-rebuild via CLI not identical:\n--- 1 ---\n%s\n--- 2 ---\n%s", dump1, dump2)
	}

	if err := runExtract(ctx, root, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no extraction model pinned") {
		t.Fatalf("extract must state the missing pin explicitly, got:\n%s", out.String())
	}
	if err := runDoctor(root, &out); err != nil {
		t.Fatal(err)
	}
}

func dumpIndex(t *testing.T, root string) string {
	t.Helper()
	eng, err := index.Open(filepath.Join(root, ".serenity", "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	s, err := index.DumpString(context.Background(), eng)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

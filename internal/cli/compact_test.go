package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
)

// buildSerenityBinary compiles the real ./cmd/serenity binary once per test
// and returns its path. Tests in this file exec the BUILT binary (not
// runCompact() in-process, not `go run`) — the Tier 2 integration
// requirement for a new CLI verb.
func buildSerenityBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "serenity")
	// internal/cli's package dir is two levels below the module root.
	cmd := exec.Command("go", "build", "-o", bin, "github.com/sirerun/serenity/cmd/serenity")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build serenity binary: %v\n%s", err, out)
	}
	return bin
}

// TestCompactCLINoConfirmExits1: the built binary's `compact` subcommand
// refuses without --confirm (RFC §7.7 — compaction is destructive to shard
// file layout and stays explicit until M2 gates it behind a disposition
// item).
func TestCompactCLINoConfirmExits1(t *testing.T) {
	requireGit(t)
	bin := buildSerenityBinary(t)
	root := t.TempDir()

	// Scaffold in-process (like TestSyncWipeRebuildViaCLI) so this relies
	// on TestMain's keychain mock — `init` execed as a separate binary
	// would hit the real OS keychain, which CI runners don't have.
	var initOut bytes.Buffer
	if err := runInit(root, &initOut); err != nil {
		t.Fatalf("init: %v\n%s", err, initOut.String())
	}

	cmd := exec.Command(bin, "-C", root, "compact")
	out, err := cmd.CombinedOutput()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected an *exec.ExitError, got %v (output: %s)", err, out)
	}
	if code := exitErr.ExitCode(); code != 1 {
		t.Fatalf("expected exit 1, got %d; output: %s", code, out)
	}
	if !strings.Contains(strings.ToLower(string(out)), "confirm") {
		t.Fatalf("expected output to mention 'confirm', got: %s", out)
	}
}

// TestCompactCLIConfirm seeds a shard family with an active claim then a
// superseding claim on the same object key, runs the built binary through
// sync -> compact --confirm -> sync, and asserts (as independent subtests)
// that the archive shard exists, the live shard holds only the resolved
// head, and the derived-index dump is byte-identical before and after
// compaction (RFC §7.7).
func TestCompactCLIConfirm(t *testing.T) {
	requireGit(t)
	bin := buildSerenityBinary(t)
	root := t.TempDir()

	var initOut bytes.Buffer
	if err := runInit(root, &initOut); err != nil {
		t.Fatalf("init: %v\n%s", err, initOut.String())
	}

	const slug, family = "acct-42", "has_balance"
	ss := store.NewShardStore(root)
	obs := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	c1 := domain.Claim{
		SubjectSlug: slug, Predicate: family, Family: family,
		Object: "100.00 usd", ObjectKey: "k1", Confidence: 0.9, State: domain.StateActive,
		Provenance: domain.Provenance{ObservedAt: obs, Actor: "machine", SourceSHA256: "src-1"},
	}
	if err := ss.Append(c1); err != nil {
		t.Fatal(err)
	}
	lines, err := ss.Lines(slug, family)
	if err != nil || len(lines) != 1 {
		t.Fatalf("seed line 1: %v %+v", err, lines)
	}
	c2 := domain.Claim{
		SubjectSlug: slug, Predicate: family, Family: family,
		Object: "150.00 usd", ObjectKey: "k1", Confidence: 0.9, State: domain.StateActive,
		Supersedes: lines[0].ID,
		Provenance: domain.Provenance{ObservedAt: obs.Add(time.Hour), Actor: "machine", SourceSHA256: "src-2"},
	}
	if err := ss.Append(c2); err != nil {
		t.Fatal(err)
	}
	lines, err = ss.Lines(slug, family)
	if err != nil || len(lines) != 2 {
		t.Fatalf("seed line 2: %v %+v", err, lines)
	}
	headID := lines[1].ID

	if out, err := exec.Command(bin, "-C", root, "sync").CombinedOutput(); err != nil {
		t.Fatalf("pre-compact sync: %v\n%s", err, out)
	}
	dumpBefore := dumpIndex(t, root)

	confirmCmd := exec.Command(bin, "-C", root, "compact", "--confirm")
	confirmOut, err := confirmCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compact --confirm: %v\n%s", err, confirmOut)
	}

	t.Run("archive-exists", func(t *testing.T) {
		archPath := ss.PathFor(slug, family+".archive")
		fi, err := os.Stat(archPath)
		if err != nil {
			t.Fatalf("archive shard missing: %v", err)
		}
		if fi.Size() == 0 {
			t.Fatal("archive shard is empty")
		}
		b, err := os.ReadFile(archPath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(b, []byte(lines[0].ID)) {
			t.Fatalf("archive shard does not contain the superseded line %q:\n%s", lines[0].ID, b)
		}
	})

	t.Run("live-heads-only", func(t *testing.T) {
		live, err := ss.Lines(slug, family)
		if err != nil {
			t.Fatal(err)
		}
		if len(live) != 1 {
			t.Fatalf("live shard has %d lines, want 1 (heads only): %+v", len(live), live)
		}
		if live[0].ID != headID {
			t.Fatalf("live shard head = %s, want %s", live[0].ID, headID)
		}
	})

	t.Run("sync-identical", func(t *testing.T) {
		if out, err := exec.Command(bin, "-C", root, "sync").CombinedOutput(); err != nil {
			t.Fatalf("post-compact sync: %v\n%s", err, out)
		}
		dumpAfter := dumpIndex(t, root)
		if dumpBefore != dumpAfter {
			t.Fatalf("post-compact sync dump differs from pre-compact:\n--- before ---\n%s\n--- after ---\n%s", dumpBefore, dumpAfter)
		}
		if dumpBefore == "" {
			t.Fatal("dump is empty — test seeded nothing observable")
		}
	})
}

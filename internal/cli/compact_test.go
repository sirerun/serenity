package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
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

// TestCompactCLINoItemExits1: the built binary's `compact` subcommand
// refuses with neither --propose nor --item (RFC §7.7 -- compaction is
// destructive to shard file layout and stays explicit, disposition-
// approved -- T2.9 replaces T0.9's original --confirm gate with this one).
func TestCompactCLINoItemExits1(t *testing.T) {
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
	if !strings.Contains(string(out), "--item") {
		t.Fatalf("expected output to mention --item, got: %s", out)
	}
}

// TestCompactCLIProposeStagesCompactItem: --propose creates a real,
// pending KindCompact disposition item and prints its id -- the item a
// human then reviews via `serenity inbox` before --item can use it.
func TestCompactCLIProposeStagesCompactItem(t *testing.T) {
	requireGit(t)
	bin := buildSerenityBinary(t)
	root := t.TempDir()

	var initOut bytes.Buffer
	if err := runInit(root, &initOut); err != nil {
		t.Fatalf("init: %v\n%s", err, initOut.String())
	}

	out, err := exec.Command(bin, "-C", root, "compact", "--propose").CombinedOutput()
	if err != nil {
		t.Fatalf("compact --propose: %v\n%s", err, out)
	}
	id := parseProposedCompactItemID(t, string(out))

	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = eng.Close() }()
	dispStore := disposition.NewStore(eng)

	item, err := dispStore.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get %s: %v", id, err)
	}
	if item.Kind != disposition.KindCompact {
		t.Fatalf("item.Kind = %q, want %q", item.Kind, disposition.KindCompact)
	}
	if item.State != disposition.StatePending {
		t.Fatalf("item.State = %q, want %q", item.State, disposition.StatePending)
	}
}

// TestCompactCLIItemRefusesUnacceptedItem: --item naming a real but still-
// pending (never disposed) compact item refuses -- staging a proposal is
// not the same as a human accepting it.
func TestCompactCLIItemRefusesUnacceptedItem(t *testing.T) {
	requireGit(t)
	bin := buildSerenityBinary(t)
	root := t.TempDir()

	var initOut bytes.Buffer
	if err := runInit(root, &initOut); err != nil {
		t.Fatalf("init: %v\n%s", err, initOut.String())
	}

	proposeOut, err := exec.Command(bin, "-C", root, "compact", "--propose").CombinedOutput()
	if err != nil {
		t.Fatalf("compact --propose: %v\n%s", err, proposeOut)
	}
	id := parseProposedCompactItemID(t, string(proposeOut))

	out, err := exec.Command(bin, "-C", root, "compact", "--item", id).CombinedOutput()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected an *exec.ExitError for an unaccepted item, got %v (output: %s)", err, out)
	}
	if code := exitErr.ExitCode(); code != 1 {
		t.Fatalf("expected exit 1, got %d; output: %s", code, out)
	}
	if !strings.Contains(string(out), "not an accepted") {
		t.Fatalf("expected output to explain the item is not accepted, got: %s", out)
	}
}

// TestCompactCLIItemAccepted seeds a shard family with an active claim
// then a superseding claim on the same object key, proposes a compact
// item and accepts it (a real Store.Dispose call, not a shortcut), runs
// the built binary through sync -> compact --item <id> -> sync, and
// asserts (as independent subtests) that the archive shard exists, the
// live shard holds only the resolved head, and the derived-index dump is
// byte-identical before and after compaction (RFC §7.7 / T2.9's own acc
// line).
func TestCompactCLIItemAccepted(t *testing.T) {
	requireGit(t)
	bin := buildSerenityBinary(t)
	root := t.TempDir()

	var initOut bytes.Buffer
	if err := runInit(root, &initOut); err != nil {
		t.Fatalf("init: %v\n%s", err, initOut.String())
	}

	configureGitIdentity(t, root)

	const slug, family = "acct-42", "has_balance"
	ss := store.NewShardStore(root)
	q := writer.NewQueue(nil)
	defer q.Close()
	obs := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	c1 := domain.Claim{
		SubjectSlug: slug, Predicate: family, Family: family,
		Object: "100.00 usd", ObjectKey: "k1", Confidence: 0.9, State: domain.StateActive,
		Provenance: domain.Provenance{ObservedAt: obs, Actor: "machine", SourceSHA256: "src-1"},
	}
	if _, _, err := writer.Shard(q, ss, c1); err != nil {
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
	if _, _, err := writer.Shard(q, ss, c2); err != nil {
		t.Fatal(err)
	}
	lines, err = ss.Lines(slug, family)
	if err != nil || len(lines) != 2 {
		t.Fatalf("seed line 2: %v %+v", err, lines)
	}
	headID := lines[1].ID

	// Canonical producer writes must be committed before a destructive pass.
	if _, err := writer.Flush(q, root); err != nil {
		t.Fatal(err)
	}

	if out, err := exec.Command(bin, "-C", root, "sync").CombinedOutput(); err != nil {
		t.Fatalf("pre-compact sync: %v\n%s", err, out)
	}
	dumpBefore := dumpIndex(t, root)

	proposeOut, err := exec.Command(bin, "-C", root, "compact", "--propose").CombinedOutput()
	if err != nil {
		t.Fatalf("compact --propose: %v\n%s", err, proposeOut)
	}
	id := parseProposedCompactItemID(t, string(proposeOut))

	// Accept it -- a real Store.Dispose call against the same index the
	// binary just wrote to, the same way a human's `serenity inbox`
	// space/accept keystroke would (RFC 0001 §8.2), not an in-process
	// shortcut around the disposition machinery.
	eng, err := providers.OpenIndex(root)
	if err != nil {
		t.Fatal(err)
	}
	dispStore := disposition.NewStore(eng)
	if _, err := dispStore.Dispose(context.Background(), id, disposition.VerdictAccept, nil, "", "human:tester", "", time.Now()); err != nil {
		t.Fatalf("accept compact item: %v", err)
	}
	if err := eng.Close(); err != nil {
		t.Fatal(err)
	}

	itemOut, err := exec.Command(bin, "-C", root, "compact", "--item", id).CombinedOutput()
	if err != nil {
		t.Fatalf("compact --item %s: %v\n%s", id, err, itemOut)
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

// parseProposedCompactItemID extracts the item id `compact --propose`
// printed ("staged compact disposition item <id> -- ..."), failing the
// test with the full output if the expected shape isn't there.
func parseProposedCompactItemID(t *testing.T, out string) string {
	t.Helper()
	const marker = "staged compact disposition item "
	i := strings.Index(out, marker)
	if i < 0 {
		t.Fatalf("propose output missing %q: %s", marker, out)
	}
	rest := out[i+len(marker):]
	id, _, ok := strings.Cut(rest, " ")
	if !ok || id == "" {
		t.Fatalf("could not parse item id from propose output: %s", out)
	}
	return id
}

// TestCompactPayloadMarshalsToEmptyObject pins CompactPayload's wire shape
// -- an empty JSON object, not null or an array -- since disposition.Item
// stores it as opaque json.RawMessage a future reader must be able to
// decode.
func TestCompactPayloadMarshalsToEmptyObject(t *testing.T) {
	b, err := json.Marshal(CompactPayload{})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "{}" {
		t.Fatalf("CompactPayload{} marshaled to %s, want {}", b)
	}
}

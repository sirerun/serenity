package serenity_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/dira/ledger"
)

// fixtureBrain copies testdata/brain-fixture's .dira ledger (T3.14's
// byte-identical copy of dira's own daemon fixture) into a fresh temp
// root and scaffolds the serenity.yml every verb requires, so both the
// CLI and the facade read a genuine on-disk ledger and the checked-in
// fixture is never written to. It returns the temp root.
func fixtureBrain(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	entries := filepath.Join(root, ".dira", "entries")
	if err := os.MkdirAll(entries, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join("..", "..", "testdata", "brain-fixture", ".dira", "entries")
	des, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("read brain-fixture entries: %v", err)
	}
	n := 0
	for _, de := range des {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(src, de.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(entries, de.Name()), data, 0o644); err != nil {
			t.Fatal(err)
		}
		n++
	}
	if n == 0 {
		t.Fatal("brain-fixture has no ledger entries; the drift corpus is empty")
	}
	if err := config.Default().Save(filepath.Join(root, config.FileName)); err != nil {
		t.Fatal(err)
	}
	return root
}

// Verbatim why_not/revisit_if for seeded constraints, with punctuation and
// a quote mark so a lossy reformatting would surface as drift.
const (
	fixtureWhyNot    = `Unbounded spend risk: "no ceiling" was rejected outright, per the Q3 freeze.`
	fixtureRevisitIf = "quarterly budget review"
)

// seedEntry writes one ledger entry file directly (ledger.Encode +
// os.WriteFile), deliberately not through direction.Store or a
// writer.Queue: pkg/serenity's tests must not import internal/writer any
// more than the package does.
func seedEntry(t *testing.T, root string, e *ledger.Entry) {
	t.Helper()
	data, err := ledger.Encode(e)
	if err != nil {
		t.Fatalf("encode %s: %v", e.ID, err)
	}
	path := filepath.Join(root, ".dira", "entries", e.ID+".md")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func appliesWhenBody(action, paramsYAML string) string {
	body := "Fixture entry for pkg/serenity tests.\n\n```serenity:applies_when\naction: " + action + "\n"
	if paramsYAML != "" {
		body += "params: " + paramsYAML + "\n"
	}
	return body + "```\n"
}

func seedConstraint(t *testing.T, root, id, action, paramsYAML string) {
	t.Helper()
	seedEntry(t, root, &ledger.Entry{
		ID: id, Kind: ledger.KindConstraint, Title: "fixture constraint " + id,
		State: ledger.StateActive, Created: "2026-09-02T00:00:00Z",
		Alternatives: []ledger.Alternative{{Option: "no ceiling", WhyNot: fixtureWhyNot, RevisitIf: fixtureRevisitIf}},
		Body:         appliesWhenBody(action, paramsYAML),
	})
}

func seedOpenQuestion(t *testing.T, root, id, action string) {
	t.Helper()
	seedEntry(t, root, &ledger.Entry{
		ID: id, Kind: ledger.KindQuestion, Title: "Do we need a second approver for this wire?",
		State: ledger.StateOpen, Created: "2026-09-02T00:00:00Z",
		Body: appliesWhenBody(action, ""),
	})
}

// buildSerenityBinary compiles ./cmd/serenity once per test that needs the
// real CLI (internal/cli/compact_test.go's pattern).
func buildSerenityBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "serenity")
	cmd := exec.Command("go", "build", "-o", bin, "github.com/sirerun/serenity/cmd/serenity")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build serenity binary: %v\n%s", err, out)
	}
	return bin
}

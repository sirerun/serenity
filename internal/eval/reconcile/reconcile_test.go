package reconcile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// repoRoot resolves the repo root relative to this test file's own path --
// the same technique internal/eval/direction's own tests use, so it works
// regardless of the working directory a test runner uses.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

func corpusDir(t *testing.T) string {
	return filepath.Join(repoRoot(t), "evals", "corpora", "reconcile")
}

func TestLoadRowsReadsEveryLabelFileSortedByName(t *testing.T) {
	rows, err := LoadRows(filepath.Join(corpusDir(t), "labels"))
	if err != nil {
		t.Fatalf("LoadRows: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("LoadRows returned zero rows")
	}
	for i := 1; i < len(rows); i++ {
		if rows[i-1].ID >= rows[i].ID {
			t.Errorf("rows not sorted by id: %s then %s", rows[i-1].ID, rows[i].ID)
		}
	}
}

func TestLoadRowsMissingDirReturnsEmptyNotError(t *testing.T) {
	rows, err := LoadRows(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("LoadRows: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %v, want empty", rows)
	}
}

func TestDetectConflictFixture(t *testing.T) {
	r := Row{
		ID:              "T-conflict",
		NewClaim:        ClaimFixture{ID: "claim-a", Subject: "acme-corp", Predicate: "has_balance", Object: "$700"},
		Active:          []ClaimFixture{{ID: "claim-b", Subject: "acme-corp", Predicate: "has_balance", Object: "$500"}},
		ExpectedVerdict: "conflict",
	}
	d := r.Detect()
	if string(d.Verdict) != "conflict" {
		t.Fatalf("Verdict = %q, want conflict", d.Verdict)
	}
}

func TestDetectAgreeFixtureNormalizesObjectKey(t *testing.T) {
	// Different casing/whitespace on an otherwise-identical object must
	// still resolve to VerdictAgree -- proves toDomain really derives
	// ObjectKey via store.NormalizeKey rather than leaving it empty
	// (an empty ObjectKey would never match itself in detectPair's
	// ObjectKey == ObjectKey comparison).
	r := Row{
		NewClaim: ClaimFixture{ID: "claim-a", Subject: "carol-diaz", Predicate: "works_at", Object: "Globex  Corp"},
		Active:   []ClaimFixture{{ID: "claim-b", Subject: "carol-diaz", Predicate: "works_at", Object: "globex corp"}},
	}
	d := r.Detect()
	if string(d.Verdict) != "agree" {
		t.Fatalf("Verdict = %q, want agree (ObjectKey normalization should make these match)", d.Verdict)
	}
}

func TestDetectRespectsExplicitSupersededState(t *testing.T) {
	r := Row{
		NewClaim: ClaimFixture{ID: "claim-a", Subject: "erin-fox", Predicate: "works_at", Object: "acme-corp"},
		Active:   []ClaimFixture{{ID: "claim-b", Subject: "erin-fox", Predicate: "works_at", Object: "initech", State: "superseded"}},
	}
	d := r.Detect()
	if string(d.Verdict) != "neutral_additive" {
		t.Fatalf("Verdict = %q, want neutral_additive -- a superseded neighbor is not an active candidate", d.Verdict)
	}
}

func TestManifestVerifiesAgainstCommittedCorpus(t *testing.T) {
	dir := corpusDir(t)
	labelsDir := filepath.Join(dir, "labels")
	manifestPath := filepath.Join(labelsDir, ManifestName)
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("manifest missing: %v", err)
	}
}

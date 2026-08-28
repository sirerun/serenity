package direction

import (
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"testing"

	"github.com/sirerun/serenity/internal/eval"
)

// corpusDir locates evals/corpora/direction/labels relative to this test
// file's own path, so it resolves regardless of the working directory a
// test runner uses.
func corpusDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "evals", "corpora", "direction", "labels")
}

func loadCorpus(t *testing.T) []Row {
	t.Helper()
	rows, err := LoadRows(corpusDir(t))
	if err != nil {
		t.Fatalf("LoadRows: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("LoadRows returned zero rows -- is evals/corpora/direction/labels/ present?")
	}
	return rows
}

// TestChecksumManifestVerifies is T3.13's checksum-pinning gate, reusing
// T1.13's exact tooling (internal/eval.VerifyManifest) unmodified per ADR-005
// -- a label file edited without re-running the manifest generator fails
// here, the same property internal/eval/checksum_test.go proves for the
// extraction corpus format.
func TestChecksumManifestVerifies(t *testing.T) {
	dir := corpusDir(t)
	manifestPath := filepath.Join(dir, ManifestName)
	if err := eval.VerifyManifest(dir, manifestPath); err != nil {
		t.Fatalf("VerifyManifest: %v\n(if you edited a row deliberately, regenerate with evals/corpora/direction/gen_manifest.go)", err)
	}
}

// TestCorpusMeetsSizeFloor is T3.13's acceptance criterion directly:
// evals/corpora/direction/ has >= 60 labeled rows with >= 10 adversarial.
func TestCorpusMeetsSizeFloor(t *testing.T) {
	rows := loadCorpus(t)
	if len(rows) < 60 {
		t.Errorf("corpus has %d rows, want >= 60", len(rows))
	}
	adversarial := 0
	for _, r := range rows {
		if r.Adversarial {
			adversarial++
		}
	}
	if adversarial < 10 {
		t.Errorf("corpus has %d adversarial rows, want >= 10", adversarial)
	}
	t.Logf("%d total rows, %d adversarial", len(rows), adversarial)
}

// TestNoDuplicateRowIDs guards the golden set's addressability: T3.16's
// report and any future adjudication pass references rows by id.
func TestNoDuplicateRowIDs(t *testing.T) {
	rows := loadCorpus(t)
	seen := make(map[string]bool, len(rows))
	for _, r := range rows {
		if r.ID == "" {
			t.Errorf("row with empty id (plan_text %q)", r.PlanText)
			continue
		}
		if seen[r.ID] {
			t.Errorf("duplicate row id %q", r.ID)
		}
		seen[r.ID] = true
	}
}

// TestValidVerdictEnum guards against a typo'd verdict string -- ADR 010
// names exactly four, no fifth.
func TestValidVerdictEnum(t *testing.T) {
	valid := map[string]bool{
		VerdictPass: true, VerdictViolated: true,
		VerdictNoApplicableConstraints: true, VerdictUnverified: true,
	}
	for _, r := range loadCorpus(t) {
		if !valid[r.ExpectedVerdict] {
			t.Errorf("row %s: expected_verdict %q is not one of the four ADR 010 verdicts", r.ID, r.ExpectedVerdict)
		}
	}
}

// TestRowContentIsPopulated is a light content-quality check: every row
// carries a real plan, a labeling rationale, and a labeler -- the minimum
// bar for the row to be reviewable, independent of whether its verdict is
// correct (that is TestExpectedVerdictMatchesReferenceEvaluator).
func TestRowContentIsPopulated(t *testing.T) {
	for _, r := range loadCorpus(t) {
		if r.PlanText == "" {
			t.Errorf("row %s: empty plan_text", r.ID)
		}
		if r.Rationale == "" {
			t.Errorf("row %s: empty rationale", r.ID)
		}
		if r.Labeler == "" {
			t.Errorf("row %s: empty labeler", r.ID)
		}
		if r.Category != "matrix" && r.Category != "adversarial" {
			t.Errorf("row %s: category %q is not matrix or adversarial", r.ID, r.Category)
		}
		if len(r.Constraints) == 0 {
			t.Errorf("row %s: empty constraint set -- every row states the constraints considered, even when none apply", r.ID)
		}
	}
}

// TestEveryAdversarialRowExpectsViolation is the acceptance-critical guard
// the team lead flagged by name: "these MUST verdict correctly (violated,
// not pass) to be useful eval data; don't seed a corpus where the
// 'adversarial' rows are mislabeled." A manipulative plan that the checker
// correctly resists is, by construction, a plan that still violates the
// constraint it tried to talk around.
func TestEveryAdversarialRowExpectsViolation(t *testing.T) {
	for _, r := range loadCorpus(t) {
		if !r.Adversarial {
			continue
		}
		if r.ExpectedVerdict != VerdictViolated {
			t.Errorf("row %s: adversarial row's expected_verdict is %q, want %q", r.ID, r.ExpectedVerdict, VerdictViolated)
		}
		if len(r.ExpectedViolations) == 0 {
			t.Errorf("row %s: adversarial row declares no expected_violations", r.ID)
		}
		if r.AdversarialKind == "" {
			t.Errorf("row %s: adversarial row has no adversarial_kind", r.ID)
		}
	}
}

// TestExpectedVerdictMatchesReferenceEvaluator is the real correctness
// gate: it independently recomputes each row's verdict from its declared
// actions and constraints (Evaluate) and asserts the result matches what
// the row claims. This is what actually catches a mislabeled row -- an
// adversarial row that merely sets expected_verdict: violated by hand,
// without its actions/params genuinely triggering a constraint, fails here.
func TestExpectedVerdictMatchesReferenceEvaluator(t *testing.T) {
	for _, r := range loadCorpus(t) {
		gotVerdict, gotViolations, gotConsidered := Evaluate(r)

		if gotVerdict != r.ExpectedVerdict {
			t.Errorf("row %s: Evaluate verdict = %q, row declares %q", r.ID, gotVerdict, r.ExpectedVerdict)
		}

		wantViolations := append([]string(nil), r.ExpectedViolations...)
		sort.Strings(wantViolations)
		if len(gotViolations) != 0 || len(wantViolations) != 0 {
			if !reflect.DeepEqual(gotViolations, wantViolations) {
				t.Errorf("row %s: Evaluate violations = %v, row declares %v", r.ID, gotViolations, wantViolations)
			}
		}

		if r.ExpectedVerdict == VerdictNoApplicableConstraints && r.ExpectedConstraintsConsidered != 0 {
			if gotConsidered != r.ExpectedConstraintsConsidered {
				t.Errorf("row %s: Evaluate considered %d constraints, row declares %d", r.ID, gotConsidered, r.ExpectedConstraintsConsidered)
			}
		}
	}
}

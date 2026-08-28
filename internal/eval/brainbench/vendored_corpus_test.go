package brainbench

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
)

// corpusDir locates evals/brainbench relative to this test file's own
// path, so it resolves regardless of the working directory a test runner
// uses -- the same technique internal/eval/direction/direction_test.go and
// internal/eval/ava_corpus_test.go use for their own corpora.
func corpusDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "evals", "brainbench")
}

// TestVendoredCorpusLoads is T1.21's acc line proved directly: this is
// what "ci runs the vendored gbrain BrainBench fixtures through serenity
// search" means in practice -- go test ./... already exercises it on
// every push (ci.yml's `test` job), independent of the separate
// evals/brainbench-trend.json artifact step (evals/brainbench/gen_trend.go)
// that publishes the same run's numbers as a CI artifact.
func TestVendoredCorpusRunsThroughSerenitySearch(t *testing.T) {
	dir := corpusDir(t)
	fixtures, err := LoadFixtures(filepath.Join(dir, "fixtures"))
	if err != nil {
		t.Fatalf("LoadFixtures: %v", err)
	}
	if len(fixtures) == 0 {
		t.Fatal("LoadFixtures returned zero fixtures -- is evals/brainbench/fixtures/ vendored?")
	}
	gold, err := LoadGold(filepath.Join(dir, "gold"))
	if err != nil {
		t.Fatalf("LoadGold: %v", err)
	}
	if len(gold) != len(fixtures) {
		t.Fatalf("got %d gold records for %d fixtures -- vendored set is not 1:1", len(gold), len(fixtures))
	}

	report, err := Evaluate(context.Background(), fixtures, gold, 10)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if report.FixturesTotal != len(fixtures) {
		t.Fatalf("FixturesTotal = %d, want %d", report.FixturesTotal, len(fixtures))
	}
	// The corpus's kta-pos/push/adversarial/multi-source categories all
	// carry should_retrieve:true turns over seeded pages (verified by
	// inspection while building this adapter) -- a run that scores zero
	// fixtures would mean the vendored set or the skip logic broke, not a
	// real result.
	if report.FixturesScored == 0 {
		t.Fatal("FixturesScored = 0 -- every fixture was skipped; vendored corpus or skip logic is broken")
	}
	if report.FixturesScored+len(report.FixturesSkipped) != report.FixturesTotal {
		t.Fatalf("scored (%d) + skipped (%d) != total (%d) -- a fixture was silently dropped",
			report.FixturesScored, len(report.FixturesSkipped), report.FixturesTotal)
	}

	for name, m := range map[string]CategoryMetrics{"overall": report.Overall} {
		if m.Precision < 0 || m.Precision > 1 {
			t.Errorf("%s.Precision = %v, out of [0,1]", name, m.Precision)
		}
		if m.Recall < 0 || m.Recall > 1 {
			t.Errorf("%s.Recall = %v, out of [0,1]", name, m.Recall)
		}
	}

	t.Logf("brainbench: %d/%d fixtures scored, %d queries, overall precision=%.4f recall=%.4f f1=%.4f",
		report.FixturesScored, report.FixturesTotal, report.Overall.Queries,
		report.Overall.Precision, report.Overall.Recall, report.Overall.F1)
	for cat, m := range report.ByCategory {
		t.Logf("  %-14s queries=%-4d precision=%.4f recall=%.4f f1=%.4f", cat, m.Queries, m.Precision, m.Recall, m.F1)
	}
}

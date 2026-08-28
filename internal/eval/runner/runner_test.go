package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/sirerun/serenity/internal/eval"
	"github.com/sirerun/serenity/internal/extract"
	"github.com/sirerun/serenity/internal/router"
)

// repoRoot resolves the repo root relative to this test file's own path,
// so the real evals/corpora/direction corpus and its checked-in cached
// fixture resolve regardless of the working directory a test runner uses.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

// buildTinyCorpus writes a minimal T1.14-shaped corpus (labels/,
// checksums.yaml, split.yaml) under t.TempDir(), using the real
// production writers (eval.WriteManifest) so the manifest is never
// hand-computed. Every label is held out -- this is a scoring-plumbing
// test, not a held-out-vs-training-split test (that is
// evals/corpora/ava's own concern).
func buildTinyCorpus(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	labelsDir := filepath.Join(dir, labelsSubdir)
	if err := os.MkdirAll(labelsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	labels := []eval.Label{
		{Span: "Ava works at Acme.", Expected: eval.ExpectedFact{Predicate: "works_at", Object: "acme"}},
		{Span: "Ava is a Staff Engineer.", Expected: eval.ExpectedFact{Predicate: "has_role", Object: "staff-engineer"}},
		{Span: "Ava prefers tea.", Expected: eval.ExpectedFact{Predicate: "prefers", Object: "tea"}},
	}
	for i, l := range labels {
		b, err := yaml.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Join(labelsDir, tinyLabelFileName(i))
		if err := os.WriteFile(name, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	manifestPath := filepath.Join(dir, manifestSubpath)
	if err := eval.WriteManifest(labelsDir, manifestPath); err != nil {
		t.Fatal(err)
	}

	split := eval.Split{HeldOut: []string{labels[0].Span, labels[1].Span, labels[2].Span}}
	sb, err := yaml.Marshal(split)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, splitSubpath), sb, 0o644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func tinyLabelFileName(i int) string {
	return "span-" + string(rune('a'+i)) + ".yaml"
}

func TestRunCachedScoresFixtureAgainstHeldOut(t *testing.T) {
	corpus := buildTinyCorpus(t)

	// Mirrors eval.LoadPredictions' expected on-disk shape
	// ({predictions: [{span, predicate, object}, ...]}); predictionsFile
	// itself is unexported in package eval, so the test builds the same
	// shape locally rather than reaching into it.
	fixture := struct {
		Predictions []eval.Prediction `yaml:"predictions"`
	}{Predictions: []eval.Prediction{
		{Span: "Ava works at Acme.", Predicate: "works_at", Object: "acme"},          // TP
		{Span: "Ava is a Staff Engineer.", Predicate: "has_role", Object: "manager"}, // wrong object: FP has_role, FN has_role
		// "Ava prefers tea." gets no prediction at all: FN prefers.
	}}
	fb, err := yaml.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join(t.TempDir(), "predictions.yaml")
	if err := os.WriteFile(fixturePath, fb, 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := Run(context.Background(), Config{
		CorpusDir:   corpus,
		Mode:        ModeCached,
		FixturePath: fixturePath,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if report.Mode != ModeCached {
		t.Errorf("Mode = %q, want %q", report.Mode, ModeCached)
	}
	if report.SpansScored != 3 {
		t.Errorf("SpansScored = %d, want 3", report.SpansScored)
	}
	if report.Spend != nil {
		t.Errorf("cached mode must not report Spend, got %+v", report.Spend)
	}
	if report.Contradiction == nil || report.Contradiction.Status == "" {
		t.Fatal("Contradiction section must always be present with a non-empty status")
	}
	if report.Contradiction.Result != nil {
		t.Error("no contradiction detector exists yet -- Contradiction.Result must be nil, not a fabricated score")
	}

	wantWorksAt := eval.PRF1{TP: 1, FP: 0, FN: 0, Precision: 1, Recall: 1, F1: 1}
	if got := report.Families["works_at"]; got != wantWorksAt {
		t.Errorf("works_at = %+v, want %+v", got, wantWorksAt)
	}
	wantHasRole := eval.PRF1{TP: 0, FP: 1, FN: 1, Precision: 0, Recall: 0, F1: 0}
	if got := report.Families["has_role"]; got != wantHasRole {
		t.Errorf("has_role = %+v, want %+v", got, wantHasRole)
	}
	wantPrefers := eval.PRF1{TP: 0, FP: 0, FN: 1, Precision: 0, Recall: 0, F1: 0}
	if got := report.Families["prefers"]; got != wantPrefers {
		t.Errorf("prefers = %+v, want %+v", got, wantPrefers)
	}
}

func TestRunCachedFailsOnTamperedCorpus(t *testing.T) {
	corpus := buildTinyCorpus(t)
	// Tamper with a label file without updating the manifest.
	target := filepath.Join(corpus, labelsSubdir, tinyLabelFileName(0))
	if err := os.WriteFile(target, []byte("span: tampered\nexpected:\n  predicate: works_at\n  object: evil\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Run(context.Background(), Config{CorpusDir: corpus, Mode: ModeCached, FixturePath: "unused"})
	if err == nil {
		t.Fatal("Run over a tampered corpus must fail manifest verification, got nil error")
	}
}

// fakeProvider is a router.Provider test double that always returns the
// same canned response and a caller-controlled CostUSD, so
// TestRunLiveStopsAtBudget can prove the aggregate-cap mechanism trips
// correctly -- independent of whether a real provider ever populates
// CostUSD (it doesn't today; see ledger.go's doc comment).
type fakeProvider struct {
	response  router.Response
	err       error
	callCount int
}

func (p *fakeProvider) Name() string         { return "fake" }
func (p *fakeProvider) ModelVersion() string { return "fake-model@v1" }
func (p *fakeProvider) Send(_ context.Context, _ string) (router.Response, error) {
	p.callCount++
	if p.err != nil {
		return router.Response{}, p.err
	}
	return p.response, nil
}

func TestRunLiveStopsAtBudget(t *testing.T) {
	corpus := buildTinyCorpus(t)

	provider := &fakeProvider{response: router.Response{
		Text:  `{"observations":[{"subject":"ava","predicate":"works_at","object":"acme","confidence":0.9}]}`,
		Usage: router.Usage{CostUSD: 1.00},
	}}
	// $1.00/call, cap $2.00: the first two calls push the running total to
	// exactly the cap, so the pre-call check ahead of the third span
	// trips before a third call is ever made.
	ledger := NewTrackingLedger(2.00)
	rt := router.New(map[router.Tier]router.Provider{router.TierLocalCheap: provider}, ledger)
	ex := extract.New(rt, "fake-model@v1", nil, extract.NewMemoryCache())

	report, err := Run(context.Background(), Config{
		CorpusDir:    corpus,
		Mode:         ModeLive,
		Extractor:    ex,
		Ledger:       ledger,
		BudgetUSD:    2.00,
		ModelVersion: "fake-model@v1",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if report.Spend == nil {
		t.Fatal("live mode must report Spend")
	}
	if !report.Spend.StoppedOnBudget {
		t.Error("StoppedOnBudget = false, want true")
	}
	if report.SpansSkipped == 0 {
		t.Error("SpansSkipped = 0, want at least 1: the budget cap should have stopped a call")
	}
	if report.SpansScored+report.SpansSkipped != 3 {
		t.Errorf("SpansScored(%d) + SpansSkipped(%d) != 3", report.SpansScored, report.SpansSkipped)
	}
	if got := report.Spend.SpentUSD; got < 2.00 {
		t.Errorf("SpentUSD = %.2f, want at least 2.00 (2 calls before the cap tripped)", got)
	}
	if provider.callCount > 2 {
		t.Errorf("provider called %d times, want at most 2 -- the third call should have been skipped before it was made", provider.callCount)
	}
}

// TestRunScoresDirectionSectionAlongsidePrimaryCorpus is T3.16's wiring
// test: Config.DirectionCorpusDir/DirectionFixturePath layer the real
// evals/corpora/direction corpus and its checked-in cached-predictions
// fixture onto the same Run call that scores the primary (ava-shaped)
// corpus, producing both sections in one Report -- one evals/report.json,
// two corpora.
func TestRunScoresDirectionSectionAlongsidePrimaryCorpus(t *testing.T) {
	corpus := buildTinyCorpus(t)

	fixture := struct {
		Predictions []eval.Prediction `yaml:"predictions"`
	}{Predictions: []eval.Prediction{
		{Span: "Ava works at Acme.", Predicate: "works_at", Object: "acme"},
		{Span: "Ava is a Staff Engineer.", Predicate: "has_role", Object: "staff-engineer"},
		{Span: "Ava prefers tea.", Predicate: "prefers", Object: "tea"},
	}}
	fb, err := yaml.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join(t.TempDir(), "predictions.yaml")
	if err := os.WriteFile(fixturePath, fb, 0o644); err != nil {
		t.Fatal(err)
	}

	root := repoRoot(t)
	report, err := Run(context.Background(), Config{
		CorpusDir:            corpus,
		Mode:                 ModeCached,
		FixturePath:          fixturePath,
		DirectionCorpusDir:   filepath.Join(root, "evals", "corpora", "direction"),
		DirectionFixturePath: filepath.Join(root, "evals", "fixtures", "direction-cached-predictions.yaml"),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if report.Families["works_at"].TP != 1 {
		t.Errorf("primary corpus scoring regressed: works_at TP = %d, want 1", report.Families["works_at"].TP)
	}
	if report.Direction == nil {
		t.Fatal("Direction section is nil, want populated")
	}
	if !report.Direction.Adversarial.AllCaught {
		t.Errorf("Direction.Adversarial.AllCaught = false, want true: %+v", report.Direction.Adversarial)
	}
	if report.Direction.RowsScored == 0 {
		t.Error("Direction.RowsScored = 0, want > 0")
	}
}

// TestRunDirectionRejectsLiveMode guards against a silent no-op: DIRECTION
// scoring has no ModeLive adapter yet (it would need a real ledger.Store
// built from this corpus's own applies_when representation, not just a
// router+extractor the way the primary corpus's live mode works), so
// requesting live mode with a direction corpus configured must fail loudly
// rather than skip scoring it.
func TestRunDirectionRejectsLiveMode(t *testing.T) {
	corpus := buildTinyCorpus(t)
	provider := &fakeProvider{response: router.Response{Text: `{"observations":[]}`}}
	ledger := NewTrackingLedger(0)
	rt := router.New(map[router.Tier]router.Provider{router.TierLocalCheap: provider}, ledger)
	ex := extract.New(rt, "fake-model@v1", nil, extract.NewMemoryCache())

	root := repoRoot(t)
	_, err := Run(context.Background(), Config{
		CorpusDir:          corpus,
		Mode:               ModeLive,
		Extractor:          ex,
		Ledger:             ledger,
		DirectionCorpusDir: filepath.Join(root, "evals", "corpora", "direction"),
	})
	if err == nil {
		t.Fatal("Run with DirectionCorpusDir set and Mode live must error, got nil")
	}
}

func TestRunLivePropagatesExtractorError(t *testing.T) {
	corpus := buildTinyCorpus(t)
	provider := &fakeProvider{err: errors.New("boom")}
	ledger := NewTrackingLedger(0)
	rt := router.New(map[router.Tier]router.Provider{router.TierLocalCheap: provider}, ledger)
	ex := extract.New(rt, "fake-model@v1", nil, extract.NewMemoryCache())

	_, err := Run(context.Background(), Config{
		CorpusDir: corpus,
		Mode:      ModeLive,
		Extractor: ex,
		Ledger:    ledger,
	})
	if err == nil {
		t.Fatal("Run must propagate a real extractor error, got nil")
	}
}

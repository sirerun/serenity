package report

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/store"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/index"
)

// fixtureClock is a settable index.Clock -- the same injectable-clock
// shape internal/cli/status_test.go's fakeStatusClock uses -- so every
// job timestamp this fixture seeds is exact rather than racing a real
// clock.
type fixtureClock struct{ t time.Time }

func (c *fixtureClock) Now() time.Time { return c.t }

// buildFixture seeds canonical claims plus a brain's *index.SQLite with
// disposition history, jobs, rebuild timing, and repo-size samples, and
// returns it alongside the fixed "now" every value below is computed
// against. Every number here was deliberately chosen to be exact in
// IEEE754 float64 arithmetic (halves, quarters, whole seconds) so
// TestReportGolden can compare Build's output against a hand-computed
// expectation without floating-point rounding noise.
func buildFixture(t *testing.T) (string, *index.SQLite, time.Time, int64) {
	t.Helper()
	root := reviewCanonicalRoot(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	clk := &fixtureClock{}
	eng, err := index.Open(filepath.Join(t.TempDir(), "index.db"), index.WithClock(clk))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = eng.Close() })

	// Four retained canonical fence claims, not extraction events.
	page := store.NewEntityPage(domain.Entity{Slug: "alice", Type: "person"})
	page.Claims = []domain.Claim{
		{ID: "c1", SubjectSlug: "alice", Family: "works_at", Predicate: "works_at", Object: "Acme", State: domain.StateActive},
		{ID: "c2", SubjectSlug: "alice", Family: "works_at", Predicate: "works_at", Object: "Oldco", State: domain.StateSuperseded, SupersededBy: "c3"},
		{ID: "c3", SubjectSlug: "alice", Family: "works_at", Predicate: "works_at", Object: "Newco", State: domain.StateActive},
		{ID: "c4", SubjectSlug: "alice", Family: "works_at", Predicate: "works_at", Object: "Wrongco", State: domain.StateRetracted},
	}
	data, err := store.NewFenceWriter(root).RenderEntity(page)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "brain", "entities", "person", "alice.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	ds := disposition.NewStore(eng)
	// item1: a rejected reconcile proposal. This is not a measured extraction correction.
	item1, err := ds.Create(ctx, disposition.KindReconcile, nil, "", now.Add(-6*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ds.Dispose(ctx, item1.ID, disposition.VerdictReject, nil, "wrong balance", "human:t", "", now.Add(-5*time.Hour)); err != nil {
		t.Fatal(err)
	}
	// item2: a reconcile item still pending, 1h old -- the reconcile
	// backlog and the queue's whole population.
	if _, err := ds.Create(ctx, disposition.KindReconcile, nil, "", now.Add(-1*time.Hour)); err != nil {
		t.Fatal(err)
	}

	// Jobs: "file" succeeded 1h ago (ingest lag 1h, no outage). "voice"
	// failed 4h ago then recovered 2h ago (ingest lag 2h, MTTR 2h).
	// "git-repo:demo" is still running, never succeeded (lag n/a).
	clk.t = now.Add(-2 * time.Hour)
	fileJob, err := eng.StartJob(ctx, "file")
	if err != nil {
		t.Fatal(err)
	}
	clk.t = now.Add(-1 * time.Hour)
	if err := eng.FinishJob(ctx, fileJob, index.JobSucceeded, nil, nil); err != nil {
		t.Fatal(err)
	}

	clk.t = now.Add(-5 * time.Hour)
	voiceFail, err := eng.StartJob(ctx, "voice")
	if err != nil {
		t.Fatal(err)
	}
	clk.t = now.Add(-4 * time.Hour)
	if err := eng.FinishJob(ctx, voiceFail, index.JobFailed, nil, errors.New("mic unavailable")); err != nil {
		t.Fatal(err)
	}
	clk.t = now.Add(-3 * time.Hour)
	voiceOK, err := eng.StartJob(ctx, "voice")
	if err != nil {
		t.Fatal(err)
	}
	clk.t = now.Add(-2 * time.Hour)
	if err := eng.FinishJob(ctx, voiceOK, index.JobSucceeded, nil, nil); err != nil {
		t.Fatal(err)
	}

	clk.t = now.Add(-10 * time.Minute)
	if _, err := eng.StartJob(ctx, "git-repo:demo"); err != nil {
		t.Fatal(err)
	}

	if err := eng.RecordRebuildTiming(ctx, 250*time.Millisecond, now.Add(-10*time.Minute)); err != nil {
		t.Fatal(err)
	}

	if err := eng.RecordRepoSizeSample(ctx, 1000, now.Add(-30*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := eng.RecordRepoSizeSample(ctx, 1300, now); err != nil {
		t.Fatal(err)
	}

	return root, eng, now, int64(len(data))
}

func floatPtr(v float64) *float64 { return &v }

// TestReportGolden is plan T5.11's acc line ("golden export on the
// fixture brain"), the Build half: every RFC section 16 field computed
// against buildFixture's known state must match this hand-computed
// expectation exactly.
func TestReportGolden(t *testing.T) {
	root, eng, now, canonicalBytes := buildFixture(t)
	got, err := Build(context.Background(), root, eng, now)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	rebuildAt := now.Add(-10 * time.Minute)
	// RebuildTiming.LastAt round-trips through RecordRebuildTiming's JSON
	// persistence (unlike every other field here, computed fresh each
	// call): compare it with time.Time.Equal, then normalize it to the
	// exact rebuildAt value/pointer so the whole-struct comparison below
	// isn't tripped up by internal time.Time representation differences
	// (monotonic reading, Location pointer identity) that Equal correctly
	// ignores but reflect.DeepEqual does not.
	if got.RebuildTiming.LastAt == nil || !got.RebuildTiming.LastAt.Equal(rebuildAt) {
		t.Fatalf("RebuildTiming.LastAt = %v, want %v", got.RebuildTiming.LastAt, rebuildAt)
	}
	got.RebuildTiming.LastAt = &rebuildAt
	want := Report{
		GeneratedAt:                  now,
		ClaimsByState:                map[string]int64{"active": 2, "superseded": 1, "retracted": 1},
		Extractions:                  nil,
		Corrections:                  nil,
		CorrectionsPer100Extractions: nil,
		Window:                       "current snapshot; queue disposal and recovery statistics use retained history; spend through generated_at",
		UnavailableReasons: map[string]string{
			"corrections_per_100_extractions":   "Extraction events and linked correction outcomes are not persisted; current claims are not an extraction denominator.",
			"trust_ladder_promotions_demotions": "Ladder transitions and completed sampled audits are not persisted.",
			"search_p95_ms":                     "Search latency samples are not persisted.",
		},
		TrustLadderPromotionsDemotions: LadderSection{
			Cells:                      []LadderCell{},
			SampledFalseAcceptanceRate: nil,
		},
		Spend: SpendSection{
			PerDayUSD:         map[string]float64{},
			MonthToDateUSD:    0,
			ProjectedMonthUSD: 0,
			CeilingUSD:        50,
		},
		IngestLagByConnector: []ConnectorLag{
			{Connector: "connector-1", HasSucceeded: true, LagSeconds: floatPtr(3600)},
			{Connector: "connector-2", HasSucceeded: false, LagSeconds: nil},
			{Connector: "connector-3", HasSucceeded: true, LagSeconds: floatPtr(7200)},
		},
		ReconcileBacklog: 1,
		DispositionQueue: QueueSection{
			Depth:                1,
			DepthAlert:           false,
			P50AgeSeconds:        floatPtr(3600),
			AgeAlert:             false,
			TimeToDisposeSeconds: floatPtr(3600),
			Abandonment:          floatPtr(0),
		},
		RepoGrowth: RepoGrowthSection{
			CurrentBytes:  canonicalBytes,
			Method:        "canonical regular-file byte delta over observed interval, normalized to 30 days; minimum interval 30 days",
			IntervalStart: timePtr(now.Add(-30 * 24 * time.Hour)), IntervalEnd: timePtr(now), ObservedDeltaBytes: intPtr(300),
			SampleCount:   2,
			BytesPerMonth: floatPtr(300),
		},
		RebuildTiming: RebuildSection{
			Never:      false,
			LastAt:     &rebuildAt,
			DurationMS: floatPtr(250),
		},
		ConnectorHealth: []ConnectorHealthEntry{
			{Connector: "connector-1", LastStatus: index.JobSucceeded, RecoveryMethod: "mean failed/interrupted completion to next successful completion; completed episodes only", MTTRSeconds: nil},
			{Connector: "connector-2", LastStatus: index.JobRunning, RecoveryMethod: "mean failed/interrupted completion to next successful completion; completed episodes only", MTTRSeconds: nil},
			{Connector: "connector-3", LastStatus: index.JobSucceeded, RecoveryMethod: "mean failed/interrupted completion to next successful completion; completed episodes only", CompletedRecoveries: 1, MTTRSeconds: floatPtr(7200)},
		},
		SearchP95MS: nil,
	}

	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("Build mismatch:\ngot:\n%s\nwant:\n%s", gotJSON, wantJSON)
	}
}

// rfcSection16Phrases maps each metric RFC 0001 section 16 names, verbatim
// or near-verbatim, to the lowercase JSON substring(s) Report's own field
// tags must contain -- plan T5.11's acc line: "every metric named in RFC
// section 16 appears".
var rfcSection16Phrases = map[string][]string{
	"ingest lag":                             {"ingest_lag"},
	"reconcile backlog":                      {"reconcile_backlog"},
	"disposition queue depth":                {"disposition_queue", "\"depth\""},
	"disposition queue ... time-to-dispose":  {"time_to_dispose"},
	"spend/day":                              {"per_day_usd"},
	"projected month":                        {"projected_month_usd"},
	"auto-action sampled false-acceptance":   {"sampled_false_acceptance_rate"},
	"repo growth/month":                      {"repo_growth", "bytes_per_month"},
	"rebuild timing":                         {"rebuild_timing"},
	"connector health/MTTR":                  {"connector_health", "mttr_seconds"},
	"search p95":                             {"search_p95_ms"},
	"claims by state":                        {"claims_by_state"},
	"corrections per 100 extractions":        {"corrections_per_100_extractions"},
	"trust-ladder cell promotions/demotions": {"trust_ladder_promotions_demotions"},
	"spend":                                  {"\"spend\""},
}

// TestReportMetricCoverage asserts every RFC section 16 metric phrase
// (rfcSection16Phrases) has a corresponding key in Report's marshaled
// JSON, built over buildFixture's fully-populated state so array-nested
// fields (e.g. connector_health[].mttr_seconds) actually render at least
// once rather than being vacuously absent because their slice was empty.
func TestReportMetricCoverage(t *testing.T) {
	root, eng, now, _ := buildFixture(t)
	r, err := Build(context.Background(), root, eng, now)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	doc := strings.ToLower(string(b))

	for phrase, substrs := range rfcSection16Phrases {
		for _, s := range substrs {
			if !strings.Contains(doc, strings.ToLower(s)) {
				t.Errorf("RFC section 16 metric %q: expected JSON to contain %q, it did not:\n%s", phrase, s, b)
			}
		}
	}
}

// TestMetricCoverageDetectsAbsence proves the substring check above isn't
// vacuously true -- the same non-vacuousness discipline
// internal/docs/threat_model_test.go applies to its own heading detector.
func TestMetricCoverageDetectsAbsence(t *testing.T) {
	doc := `{"claims_by_state": {"active": 1}}`
	if strings.Contains(doc, "search_p95_ms") {
		t.Fatal("test setup broken: substring unexpectedly present")
	}
}

// failTransport errors on every RoundTrip and counts how many times it
// was invoked.
type failTransport struct{ calls int }

func (f *failTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	f.calls++
	return nil, errors.New("report: unexpected network call")
}

// TestReportNoNetwork is plan T5.11's acc line, the no-network half:
// "nothing is transmitted anywhere (no network in the code path, asserted
// by a no-network test)". http.DefaultTransport is replaced with a
// RoundTripper that errors loudly on any call for the duration of this
// test; Build and MeasureRepoSize -- the two exported entry points a
// caller drives to produce a report -- both run against real fixture
// state and must trip it zero times.
func TestReportNoNetwork(t *testing.T) {
	trap := &failTransport{}
	prev := http.DefaultTransport
	http.DefaultTransport = trap
	defer func() { http.DefaultTransport = prev }()

	// Prove the trap actually intercepts before trusting a zero count
	// below.
	if _, err := http.Get("http://127.0.0.1:1/report-no-network-canary"); err == nil {
		t.Fatal("trapped transport did not error on a real request")
	}
	if trap.calls != 1 {
		t.Fatalf("canary request: trap.calls = %d, want 1", trap.calls)
	}
	trap.calls = 0

	root, eng, now, _ := buildFixture(t)
	if _, err := Build(context.Background(), root, eng, now); err != nil {
		t.Fatalf("Build: %v", err)
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "brain"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := MeasureRepoSize(dir); err != nil {
		t.Fatalf("MeasureRepoSize: %v", err)
	}

	if trap.calls != 0 {
		t.Fatalf("report generation made %d unexpected network call(s)", trap.calls)
	}
}

func timePtr(t time.Time) *time.Time { return &t }
func intPtr(n int64) *int64          { return &n }
func reviewCanonicalRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := config.Default().Save(filepath.Join(root, config.FileName)); err != nil {
		t.Fatal(err)
	}
	return root
}

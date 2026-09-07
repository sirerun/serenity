package report

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/index"
)

type reviewMetricClock struct{ at time.Time }

func (c *reviewMetricClock) Now() time.Time { return c.at }

var reviewMetricNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

func reviewMetricDB(t *testing.T, opts ...index.Option) *index.SQLite {
	t.Helper()
	eng, err := index.Open(filepath.Join(t.TempDir(), "index.db"), opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := eng.Close(); err != nil {
			t.Error(err)
		}
	})
	return eng
}

func reviewMetricJSON(t *testing.T, eng *index.SQLite) map[string]any {
	t.Helper()
	r, err := Build(context.Background(), reviewCanonicalRoot(t), eng, reviewMetricNow)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestReviewReportCorrectionsDoNotUseCurrentClaimsAsExtractions(t *testing.T) {
	ctx := context.Background()
	eng := reviewMetricDB(t)
	if err := eng.UpsertClaim(ctx, domain.Claim{ID: "review-claim", SubjectSlug: "people/review", Predicate: "works_at", Family: "works_at", Object: "Example", ObjectKey: "example", State: domain.StateActive, Confidence: 0.9}); err != nil {
		t.Fatal(err)
	}
	ds := disposition.NewStore(eng)
	item, err := ds.Create(ctx, disposition.KindReconcile, json.RawMessage(`{"existing_claim_id":"review-claim","proposed":"Another employer"}`), "", reviewMetricNow.Add(-2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ds.Dispose(ctx, item.ID, disposition.VerdictReject, nil, "new claim is mistaken", "human:fixture", "review-rejection", reviewMetricNow.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	doc := reviewMetricJSON(t, eng)
	rate, ok := doc["corrections_per_100_extractions"]
	if !ok || rate != nil {
		t.Fatalf("unobserved extraction denominator must yield explicit null rate, got %v (present=%t)", rate, ok)
	}
	// There are no extraction-event records in this fixture. Current claims
	// cannot supply an extraction total even when a human rejected a conflict.
	if count, ok := doc["extractions"]; ok && count != nil && count != float64(0) {
		t.Errorf("indexed claim fabricated an extraction count: %v", count)
	}
}

func TestReviewReportFiltersFutureSpend(t *testing.T) {
	eng := reviewMetricDB(t)
	for _, row := range []index.SpendRow{
		{ID: "past", CostUSD: 2, OccurredAt: reviewMetricNow.Add(-time.Hour)},
		{ID: "future", CostUSD: 90, OccurredAt: reviewMetricNow.Add(24 * time.Hour)},
	} {
		if err := eng.RecordSpend(context.Background(), row); err != nil {
			t.Fatal(err)
		}
	}
	doc := reviewMetricJSON(t, eng)
	spendDoc, ok := doc["spend"].(map[string]any)
	if !ok {
		t.Fatal("missing spend object")
	}
	if spendDoc["month_to_date_usd"] != float64(2) {
		t.Errorf("future spend included: %v", spendDoc)
	}
	if v, ok := spendDoc["projected_month_usd"].(float64); !ok || math.Abs(v-2.0/7*30) > 1e-9 {
		t.Errorf("projection must use only observed spend, got %v", spendDoc["projected_month_usd"])
	}
	daily, ok := spendDoc["per_day_usd"].(map[string]any)
	if !ok {
		t.Fatal("missing daily spend")
	}
	if _, ok := daily["2026-09-08"]; ok {
		t.Errorf("future day appears in as-of report: %v", daily)
	}
}

func TestReviewReportOverlappingJobsDoNotInventRecovery(t *testing.T) {
	clk := &reviewMetricClock{at: reviewMetricNow.Add(-2 * time.Hour)}
	eng := reviewMetricDB(t, index.WithClock(clk))
	ctx := context.Background()
	failed, err := eng.StartJob(ctx, "file")
	if err != nil {
		t.Fatal(err)
	}
	clk.at = reviewMetricNow.Add(-time.Hour)
	successful, err := eng.StartJob(ctx, "file")
	if err != nil {
		t.Fatal(err)
	}
	clk.at = reviewMetricNow.Add(-30 * time.Minute)
	if err := eng.FinishJob(ctx, successful, index.JobSucceeded, nil, nil); err != nil {
		t.Fatal(err)
	}
	clk.at = reviewMetricNow
	if err := eng.FinishJob(ctx, failed, index.JobFailed, nil, errors.New("fixture failure")); err != nil {
		t.Fatal(err)
	}
	doc := reviewMetricJSON(t, eng)
	entries, ok := doc["connector_health"].([]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("want one connector health row: %v", doc["connector_health"])
	}
	entry, ok := entries[0].(map[string]any)
	if !ok {
		t.Fatal("invalid health row")
	}
	if mttr, ok := entry["mttr_seconds"]; !ok || mttr != nil {
		t.Fatalf("success completed BEFORE failure, so no observed recovery exists; got %v", entry)
	}
}

// Reasons may live beside a metric or in a centralized unavailable-reasons
// map. Assert evidence in the exported object, not only source comments.
func reviewHasReason(v any, terms ...string) bool {
	switch x := v.(type) {
	case string:
		s := strings.ToLower(x)
		for _, term := range terms {
			if !strings.Contains(s, term) {
				return false
			}
		}
		return true
	case map[string]any:
		for _, v := range x {
			if reviewHasReason(v, terms...) {
				return true
			}
		}
	case []any:
		for _, v := range x {
			if reviewHasReason(v, terms...) {
				return true
			}
		}
	}
	return false
}

func TestReviewReportUnavailableReasonsTravelWithExport(t *testing.T) {
	doc := reviewMetricJSON(t, reviewMetricDB(t))
	for _, terms := range [][]string{{"extract"}, {"ladder"}, {"search"}} {
		if !reviewHasReason(doc, terms...) {
			t.Errorf("no exported explanation of absent %s observations; bare zero/null is ambiguous", terms[0])
		}
	}
}

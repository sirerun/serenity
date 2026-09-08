package importbudget

import (
	"math"
	"testing"
)

func validReport() Report {
	r := Report{Version: Version, Revision: "abc", Environment: "fixture", CorpusSHA256: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", CacheVersion: "fixture-v1", Messages: 10, Claims: 10, Vectors: 12, CacheHits: 10, TotalSeconds: 100}
	for _, name := range StageNames {
		n := r.Messages
		if name == "embed" {
			n = r.Vectors
		}
		r.Stages = append(r.Stages, Stage{Name: name, Seconds: 1, Items: n})
	}
	return r
}
func TestBudgetGate(t *testing.T) {
	baseline := validReport()
	for _, tc := range []struct {
		name      string
		seconds   float64
		wantError bool
	}{{"faster", 99, false}, {"below_regression", 119.99, false}, {"twenty_percent", 120, true}, {"regression", 121, true}, {"four_hours", 14400, true}, {"nan", math.NaN(), true}, {"infinite", math.Inf(1), true}} {
		t.Run(tc.name, func(t *testing.T) {
			r := validReport()
			r.TotalSeconds = tc.seconds
			if got := Check(r, &baseline); (got != nil) != tc.wantError {
				t.Fatalf("Check=%v", got)
			}
		})
	}
	if err := Check(baseline, nil); err != nil {
		t.Fatal(err)
	}
}
func TestRejectsPartialOrIncompatibleRuns(t *testing.T) {
	for _, mode := range []string{"zero_claims", "cache_miss", "model_call", "missing_stage", "zero_stage", "different_corpus", "different_environment", "different_cache", "invalid_baseline"} {
		t.Run(mode, func(t *testing.T) {
			r, previous := validReport(), validReport()
			switch mode {
			case "zero_claims":
				r.Claims = 0
			case "cache_miss":
				r.CacheHits--
			case "model_call":
				r.ModelCalls = 1
			case "missing_stage":
				r.Stages = r.Stages[:6]
			case "zero_stage":
				r.Stages[0].Seconds = 0
			case "different_corpus":
				r.CorpusSHA256 = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			case "different_environment":
				r.Environment = "other"
			case "different_cache":
				r.CacheVersion = "v2"
			case "invalid_baseline":
				previous.Claims = 0
			}
			if err := Check(r, &previous); err == nil {
				t.Fatal("accepted invalid report")
			}
		})
	}
}

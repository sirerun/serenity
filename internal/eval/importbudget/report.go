// Package importbudget validates and compares measured offline ingest runs.
package importbudget

import (
	"fmt"
	"math"
)

const Version = 1

var StageNames = []string{"poll", "store_sources", "chunk_extract", "reconcile", "write_claims", "index", "embed"}

type Stage struct {
	Name    string  `json:"name"`
	Seconds float64 `json:"seconds"`
	Items   int     `json:"items"`
}

type Report struct {
	Version      int     `json:"version"`
	Revision     string  `json:"revision"`
	Environment  string  `json:"environment"`
	CorpusSHA256 string  `json:"corpus_sha256"`
	CacheVersion string  `json:"cache_version"`
	Messages     int     `json:"messages"`
	Claims       int     `json:"claims"`
	Vectors      int     `json:"vectors"`
	CacheHits    int     `json:"cache_hits"`
	ModelCalls   int     `json:"model_calls"`
	TotalSeconds float64 `json:"total_seconds"`
	Stages       []Stage `json:"stages"`
}

// Validate refuses empty/partial runs and non-finite timings before comparing
// performance. The test harness checks persisted row identities separately.
func Validate(r Report) error {
	if r.Version != Version || r.Revision == "" || r.Environment == "" || len(r.CorpusSHA256) != 64 || r.CacheVersion == "" {
		return fmt.Errorf("import budget: missing or unsupported run identity")
	}
	if r.Messages <= 0 || r.Claims != r.Messages || r.Vectors < r.Messages || r.CacheHits != r.Messages || r.ModelCalls != 0 {
		return fmt.Errorf("import budget: incomplete counts or live model calls")
	}
	if math.IsNaN(r.TotalSeconds) || math.IsInf(r.TotalSeconds, 0) || r.TotalSeconds <= 0 {
		return fmt.Errorf("import budget: invalid total duration")
	}
	if len(r.Stages) != len(StageNames) {
		return fmt.Errorf("import budget: missing stages")
	}
	sum := 0.0
	for i, s := range r.Stages {
		if s.Name != StageNames[i] || (s.Name != "embed" && s.Items != r.Messages) || (s.Name == "embed" && s.Items != r.Vectors) || math.IsNaN(s.Seconds) || math.IsInf(s.Seconds, 0) || s.Seconds <= 0 {
			return fmt.Errorf("import budget: invalid stage %d", i)
		}
		sum += s.Seconds
	}
	if sum > r.TotalSeconds*1.001 {
		return fmt.Errorf("import budget: stage durations exceed total")
	}
	return nil
}

// Check compares only compatible workloads on the same runner class. A missing
// prior run is an explicit bootstrap, still subject to the four-hour ceiling.
func Check(current Report, previous *Report) error {
	if err := Validate(current); err != nil {
		return err
	}
	if current.TotalSeconds >= 4*60*60 {
		return fmt.Errorf("import budget: run took %.3fs; must finish below four hours", current.TotalSeconds)
	}
	if previous == nil {
		return nil
	}
	if err := Validate(*previous); err != nil {
		return fmt.Errorf("import budget: previous run: %w", err)
	}
	if current.Environment != previous.Environment || current.CorpusSHA256 != previous.CorpusSHA256 || current.CacheVersion != previous.CacheVersion || current.Messages != previous.Messages {
		return fmt.Errorf("import budget: incompatible baseline; explicit baseline migration required")
	}
	if current.TotalSeconds >= previous.TotalSeconds*1.2 {
		return fmt.Errorf("import budget: %.3fs is at least 20%% slower than previous %.3fs", current.TotalSeconds, previous.TotalSeconds)
	}
	return nil
}

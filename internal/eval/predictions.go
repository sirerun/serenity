package eval

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// predictionsFile is the on-disk shape LoadPredictions reads: a flat list
// of Prediction under a top-level "predictions" key. This is the frozen
// cached-mode input format the plan T1.22 eval runner scores on every
// push (internal/eval/runner.ModeCached): a checked-in snapshot of
// extractor output that exercises Score/ContradictionRecall and the
// report-generation code path with zero network calls. It carries no
// provenance beyond the (Span, Predicate, Object) triple Score already
// keys on, matching Prediction's fields exactly (yaml.v3's default
// lowercased-field-name matching, same as Label's tagged fields).
type predictionsFile struct {
	Predictions []Prediction `yaml:"predictions"`
}

// LoadPredictions reads a predictionsFile-shaped YAML fixture.
func LoadPredictions(path string) ([]Prediction, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("eval: read predictions file %s: %w", path, err)
	}
	var f predictionsFile
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("eval: parse predictions file %s: %w", path, err)
	}
	return f.Predictions, nil
}

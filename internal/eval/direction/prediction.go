package direction

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Prediction is one classifier's verdict for a golden Row -- the shape a
// T1.22-style cached-predictions fixture holds for this corpus (T3.16),
// mirroring what a real check_plan call returns for a row's declared plan
// (RFC 0001 SS8.3: a verdict, keyed here by row id since DIRECTION scores
// exactly one verdict per row, never a set of facts per span the way
// internal/eval.Prediction's span/predicate/object triple does).
type Prediction struct {
	RowID   string `yaml:"row_id"`
	Verdict string `yaml:"verdict"`
}

// predictionsFile mirrors internal/eval's predictionsFile shape
// (predictions.go) for this corpus's own record type: a flat list under a
// top-level "predictions" key.
type predictionsFile struct {
	Predictions []Prediction `yaml:"predictions"`
}

// LoadPredictions reads a predictionsFile-shaped YAML fixture.
func LoadPredictions(path string) ([]Prediction, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("direction: read predictions file %s: %w", path, err)
	}
	var f predictionsFile
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("direction: parse predictions file %s: %w", path, err)
	}
	return f.Predictions, nil
}

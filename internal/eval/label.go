// Package eval implements the RFC 0001 §16 / ADR-005 held-out golden-set
// evaluator: the label format, checksum pinning, per-family precision/
// recall/F1 scoring, and contradiction-detection recall. It never reads an
// extractor's confidence to decide labels, and it never runs a production
// extractor itself -- callers hand it Predictions produced elsewhere (an
// eval workflow, or a fake extractor in a test) and this package purely
// scores them against the golden set.
package eval

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// ExpectedFact is the ground-truth predicate/object/valid-window a labeler
// assigned to a span (ADR-005).
type ExpectedFact struct {
	Predicate string `yaml:"predicate"`
	Object    string `yaml:"object"`
	ValidFrom string `yaml:"valid_from,omitempty"`
	ValidTo   string `yaml:"valid_to,omitempty"`
}

// Label is one held-out golden record (ADR-005): a source span, the fact a
// labeler assigned to it, which labeler produced it, and whether a
// maintainer adjudicated a disagreement between the two independent
// labeling passes. Family is deliberately NOT a label field: family and
// predicate are 1:1 in the seed vocabulary (internal/store/fence.go,
// internal/config/config.go's Default()), so scorers group by
// Expected.Predicate instead of carrying a redundant field that could
// silently disagree with it.
type Label struct {
	Span        string       `yaml:"span"`
	Expected    ExpectedFact `yaml:"expected"`
	Labeler     string       `yaml:"labeler"`
	Adjudicated bool         `yaml:"adjudicated"`
}

// LoadLabels reads every *.yaml file directly under dir (non-recursive) as
// one Label record per file -- ADR-005's "one record per span" -- and
// returns them in filename-sorted order for determinism. A missing or
// empty directory yields an empty, non-nil slice and no error, so callers
// can treat "no labels yet" the same as "no labels found".
//
// Unrecognized YAML fields are silently ignored (yaml.v3's default decode
// behavior), so a corpus can carry forward-compatible metadata (labeling
// notes, reviewer ids, etc.) without breaking an older harness version.
func LoadLabels(dir string) ([]Label, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Label{}, nil
		}
		return nil, fmt.Errorf("eval: read labels dir %s: %w", dir, err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	labels := make([]Label, 0, len(names))
	for _, name := range names {
		path := filepath.Join(dir, name)
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("eval: read label file %s: %w", path, err)
		}
		var l Label
		if err := yaml.Unmarshal(b, &l); err != nil {
			return nil, fmt.Errorf("eval: parse label file %s: %w", path, err)
		}
		labels = append(labels, l)
	}
	return labels, nil
}

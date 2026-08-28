package eval

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Split names which spans are held out for evaluation. RFC 0001 §16
// requires golden sets to be "held-out ... never trained-on or
// tuned-against in the same milestone that builds the extractor"; this is
// the machine-checkable record of that boundary. HeldOut lists Label.Span
// values -- spans, not file paths, because a span is the stable identifier
// a label round-trips through LoadLabels regardless of which file it
// happens to live in.
type Split struct {
	HeldOut []string `yaml:"held_out"`
}

// LoadSplit reads a split file: a YAML document with a top-level held_out
// list of span identifiers.
func LoadSplit(path string) (Split, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Split{}, fmt.Errorf("eval: read split file %s: %w", path, err)
	}
	var s Split
	if err := yaml.Unmarshal(b, &s); err != nil {
		return Split{}, fmt.Errorf("eval: parse split file %s: %w", path, err)
	}
	return s, nil
}

// Filter partitions labels into the held-out set (labels whose Span is
// named in s.HeldOut) and the rest, preserving the input order within each
// half. A span named in the split file that matches no label is silently
// ignored -- the split file is allowed to name a superset (e.g. spans
// reserved for a future corpus revision that have not been labeled yet).
func (s Split) Filter(labels []Label) (heldOut, rest []Label) {
	held := make(map[string]bool, len(s.HeldOut))
	for _, span := range s.HeldOut {
		held[span] = true
	}
	for _, l := range labels {
		if held[l.Span] {
			heldOut = append(heldOut, l)
		} else {
			rest = append(rest, l)
		}
	}
	return heldOut, rest
}

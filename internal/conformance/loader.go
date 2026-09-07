package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// LoadTranscripts reads every *.json transcript file directly under dir
// (a protocol's own testdata/conformance/<protocol>/ directory), sorted by
// filename so a report's ordering is stable across runs. Non-.json files
// (MANIFEST, LICENSE, a *.go generator script) are skipped, mirroring
// fixtures_test.go's own TestConformanceTranscriptsLoadAndAreWellFormed
// iteration.
func LoadTranscripts(dir string) ([]Transcript, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("conformance: read dir %s: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	transcripts := make([]Transcript, 0, len(names))
	for _, name := range names {
		tr, err := LoadTranscript(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		transcripts = append(transcripts, tr)
	}
	return transcripts, nil
}

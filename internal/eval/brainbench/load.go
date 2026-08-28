package brainbench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LoadFixtures reads every *.fixture.json file directly under dir
// (non-recursive) and returns them in filename-sorted order for
// determinism. A missing or empty directory yields an empty, non-nil
// slice and no error.
func LoadFixtures(dir string) ([]Fixture, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Fixture{}, nil
		}
		return nil, fmt.Errorf("brainbench: read fixtures dir %s: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".fixture.json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	fixtures := make([]Fixture, 0, len(names))
	for _, name := range names {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("brainbench: read %s: %w", name, err)
		}
		var f Fixture
		if err := json.Unmarshal(b, &f); err != nil {
			return nil, fmt.Errorf("brainbench: parse %s: %w", name, err)
		}
		if f.FixtureID == "" {
			return nil, fmt.Errorf("brainbench: %s has no fixture_id", name)
		}
		fixtures = append(fixtures, f)
	}
	return fixtures, nil
}

// LoadGold reads every *.gold.json file directly under dir and returns
// them keyed by FixtureID, the same join key Fixture carries -- a gold
// file's own filename is never load-bearing.
func LoadGold(dir string) (map[string]Gold, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Gold{}, nil
		}
		return nil, fmt.Errorf("brainbench: read gold dir %s: %w", dir, err)
	}

	gold := make(map[string]Gold)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".gold.json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("brainbench: read %s: %w", e.Name(), err)
		}
		var g Gold
		if err := json.Unmarshal(b, &g); err != nil {
			return nil, fmt.Errorf("brainbench: parse %s: %w", e.Name(), err)
		}
		if g.FixtureID == "" {
			return nil, fmt.Errorf("brainbench: %s has no fixture_id", e.Name())
		}
		if _, dup := gold[g.FixtureID]; dup {
			return nil, fmt.Errorf("brainbench: duplicate gold for fixture_id %q (from %s)", g.FixtureID, e.Name())
		}
		gold[g.FixtureID] = g
	}
	return gold, nil
}

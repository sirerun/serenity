package report

import (
	"fmt"
	"path/filepath"

	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/store"
)

// canonicalClaimCounts counts retained history once per entity/family/id.
// Shards own shard families; their rendered fence heads are ignored. A
// superseding line marks its predecessor superseded even if the immutable
// predecessor still says active. A retraction takes precedence over that link.
func canonicalClaimCounts(root string, cfg *config.Config) (map[string]int64, error) {
	counts := map[string]int64{"active": 0, "superseded": 0, "retracted": 0}
	pages, err := filepath.Glob(filepath.Join(root, "brain", "entities", "*", "*.md"))
	if err != nil {
		return nil, err
	}
	fw := store.NewFenceWriter(root)
	seen := map[string]bool{}
	for _, path := range pages {
		page, err := fw.ParseEntity(path)
		if err != nil {
			return nil, err
		}
		for _, c := range page.Claims {
			if cfg.TierOf(c.Family) == domain.TierShard {
				continue
			}
			key := page.Entity.Slug + "\x00" + c.Family + "\x00" + c.ID
			if seen[key] {
				continue
			}
			seen[key] = true
			if err := countClaimState(counts, c.State); err != nil {
				return nil, err
			}
		}
	}
	ss := store.NewShardStore(root)
	slugs, err := ss.Slugs()
	if err != nil {
		return nil, err
	}
	for _, slug := range slugs {
		families, err := ss.Families(slug)
		if err != nil {
			return nil, err
		}
		for _, family := range families {
			if cfg.TierOf(family) != domain.TierShard {
				continue
			}
			lines, err := ss.Lines(slug, family)
			if err != nil {
				return nil, err
			}
			states := map[string]domain.State{}
			superseded := map[string]bool{}
			for _, c := range lines {
				if c.Supersedes != "" {
					superseded[c.Supersedes] = true
				}
				// Repeated identical claims can occur after merging append-only files.
				old := states[c.ID]
				if old == domain.StateRetracted {
					continue
				}
				if old == domain.StateSuperseded && c.State == domain.StateActive {
					continue
				}
				states[c.ID] = c.State
			}
			for id, state := range states {
				if state == domain.StateActive && superseded[id] {
					state = domain.StateSuperseded
				}
				if err := countClaimState(counts, state); err != nil {
					return nil, err
				}
			}
		}
	}
	return counts, nil
}

func countClaimState(counts map[string]int64, state domain.State) error {
	switch state {
	case domain.StateActive, domain.StateSuperseded, domain.StateRetracted:
		counts[string(state)]++
		return nil
	default:
		return fmt.Errorf("unknown canonical claim state %q", state)
	}
}

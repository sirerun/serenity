package config

import (
	"testing"

	"github.com/sirerun/serenity/internal/domain"
)

// TestDefaultSeedsControlledVocabulary pins the exact predicate vocabulary
// RFC §7.2 requires at install: the seed set every writer enforces (T0.8)
// and that is extensible only via serenity.yml + migration, never ad hoc.
func TestDefaultSeedsControlledVocabulary(t *testing.T) {
	want := map[string]domain.Tier{
		"works_at":           domain.TierFence,
		"has_role":           domain.TierFence,
		"owns_account":       domain.TierFence,
		"has_balance":        domain.TierShard,
		"has_condition":      domain.TierFence,
		"takes_medication":   domain.TierFence,
		"prefers":            domain.TierFence,
		"committed_to":       domain.TierFence,
		"deadline_on":        domain.TierFence,
		"relates_to":         domain.TierFence,
		"belongs_to_project": domain.TierFence,
		"said":               domain.TierFence,
		"costs":              domain.TierShard,
	}

	cfg := Default()
	if len(cfg.Families) != len(want) {
		t.Fatalf("seed vocabulary has %d families, want %d: %v", len(cfg.Families), len(want), cfg.FamilyNames())
	}
	for name, tier := range want {
		f, ok := cfg.Families[name]
		if !ok {
			t.Fatalf("seed vocabulary missing predicate %q", name)
		}
		if f.Tier != tier {
			t.Fatalf("predicate %q tier = %s, want %s", name, f.Tier, tier)
		}
	}
}

// TestDefaultSeedsOpenRouterProvider pins ADR 013's install-time default:
// a brand new brain's serenity.yml selects OpenRouter as the explicit
// models.provider once a model is pinned (this does not un-skip the "no
// model pinned" none@v0 default -- see TestDefaultSeedsControlledVocabulary
// and internal/providers' explicit-skip contract).
func TestDefaultSeedsOpenRouterProvider(t *testing.T) {
	if got := Default().Models.Provider; got != "openrouter" {
		t.Fatalf("Default().Models.Provider = %q, want %q", got, "openrouter")
	}
}

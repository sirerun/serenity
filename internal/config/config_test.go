package config

import (
	"os"
	"testing"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/ladder"
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

// TestDefaultSeedsLadderPolicyPriors pins RFC §10.3's own published priors
// as config.Default's ladder values (T2.10) -- T2.11's calibration sweep
// (deps: [T2.10]) asserts config.Default's ladder values equal its own
// evidence-backed report, so this is the seam that later comparison
// depends on staying wired.
func TestDefaultSeedsLadderPolicyPriors(t *testing.T) {
	want := *ladder.DefaultConfig()
	got := Default().Ladder

	if got.Default != want.Default {
		t.Fatalf("Default().Ladder.Default = %+v, want %+v", got.Default, want.Default)
	}
	if got.CorrelationGuards != want.CorrelationGuards {
		t.Fatalf("Default().Ladder.CorrelationGuards = %+v, want %+v", got.CorrelationGuards, want.CorrelationGuards)
	}
	if len(got.PerCell) != len(want.PerCell) {
		t.Fatalf("Default().Ladder.PerCell has %d entries, want %d", len(got.PerCell), len(want.PerCell))
	}
	for cell, policy := range want.PerCell {
		if got.PerCell[cell] != policy {
			t.Fatalf("Default().Ladder.PerCell[%q] = %+v, want %+v", cell, got.PerCell[cell], policy)
		}
	}
	if len(got.NeverAutomate) != len(want.NeverAutomate) {
		t.Fatalf("Default().Ladder.NeverAutomate = %v, want %v", got.NeverAutomate, want.NeverAutomate)
	}
}

// TestConfigRoundTripsLadderThroughYAML proves a full serenity.yml
// save/load cycle preserves the ladder section byte-for-byte in value.
func TestConfigRoundTripsLadderThroughYAML(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/serenity.yml"

	cfg := Default()
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Ladder.Default != cfg.Ladder.Default {
		t.Fatalf("round-tripped Ladder.Default = %+v, want %+v", loaded.Ladder.Default, cfg.Ladder.Default)
	}
	if loaded.Ladder.CorrelationGuards != cfg.Ladder.CorrelationGuards {
		t.Fatalf("round-tripped Ladder.CorrelationGuards = %+v, want %+v", loaded.Ladder.CorrelationGuards, cfg.Ladder.CorrelationGuards)
	}
}

// TestLoadLegacyConfigWithoutLadderSectionLeavesLadderZeroValue proves a
// pre-T2.10 serenity.yml (no "ladder:" key at all) still loads cleanly --
// backward compatibility, the same posture ADR 013 established for the
// Provider field.
func TestLoadLegacyConfigWithoutLadderSectionLeavesLadderZeroValue(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/serenity.yml"
	legacy := "version: 1\nmodels:\n  embedding: none@v0\n  extraction: none@v0\n  composer: none@v0\nindex:\n  engine: sqlite\nfamilies: {}\n"
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy fixture: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load legacy config: unexpected error: %v", err)
	}
	if loaded.Ladder.Default != (ladder.CellPolicy{}) {
		t.Fatalf("Load legacy config: Ladder.Default = %+v, want zero value", loaded.Ladder.Default)
	}
	if loaded.Ladder.CorrelationGuards != (ladder.CorrelationGuards{}) {
		t.Fatalf("Load legacy config: Ladder.CorrelationGuards = %+v, want zero value", loaded.Ladder.CorrelationGuards)
	}
	if len(loaded.Ladder.PerCell) != 0 || len(loaded.Ladder.NeverAutomate) != 0 {
		t.Fatalf("Load legacy config: Ladder = %+v, want empty PerCell/NeverAutomate", loaded.Ladder)
	}
}

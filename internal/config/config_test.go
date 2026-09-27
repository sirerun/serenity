package config

import (
	"os"
	"strings"
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

func TestLoadServerMaxInFlightCalls(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/serenity.yml"
	contents := "version: 1\nmodels:\n  embedding: none@v0\n  extraction: none@v0\n  composer: none@v0\nindex:\n  engine: sqlite\nserver:\n  max_in_flight_calls: 17\nfamilies: {}\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load config: %v", err)
	}
	if loaded.Server.MaxInFlightCalls != 17 {
		t.Fatalf("Server.MaxInFlightCalls = %d, want 17", loaded.Server.MaxInFlightCalls)
	}
}

// TestLoadRejectsUnknownTopLevelKey pins SEC-H05's config half (ADR 018
// decision 3): serenity.yml is synced through the brain remote, so a key
// the schema does not know must fail Load and name itself rather than be
// silently accepted.
func TestLoadRejectsUnknownTopLevelKey(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/serenity.yml"
	contents := "version: 1\nmodels:\n  embedding: none@v0\n  extraction: none@v0\n  composer: none@v0\nindex:\n  engine: sqlite\nfamilies: {}\nexfil_hook: /bin/true\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load accepted a serenity.yml with an unknown top-level key; want an error naming it")
	}
	if !strings.Contains(err.Error(), "unknown key") || !strings.Contains(err.Error(), "exfil_hook") {
		t.Fatalf("Load error = %q, want it to contain \"unknown key\" and name \"exfil_hook\"", err)
	}
}

// TestLoadRejectsUnknownNestedKey proves strictness applies at every
// level, not only the top: a misspelled server.allow_lan must not decode
// to "allow_lan unset" and silently keep the loopback default while the
// operator believes they configured LAN exposure (or vice versa).
func TestLoadRejectsUnknownNestedKey(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/serenity.yml"
	contents := "version: 1\nmodels:\n  embedding: none@v0\n  extraction: none@v0\n  composer: none@v0\nindex:\n  engine: sqlite\nserver:\n  bind: \"127.0.0.1:0\"\n  allow_lann: true\nfamilies: {}\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load accepted a serenity.yml with an unknown nested key; want an error naming it")
	}
	if !strings.Contains(err.Error(), "unknown key") || !strings.Contains(err.Error(), "allow_lann") {
		t.Fatalf("Load error = %q, want it to contain \"unknown key\" and name \"allow_lann\"", err)
	}
}

// TestLoadRejectsUnknownConnectorKind proves the connectors section is
// typed too: a connector kind the build does not know (which would have
// been silently ignored by the old untyped map) is an error naming the
// key path.
func TestLoadRejectsUnknownConnectorKind(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/serenity.yml"
	contents := "version: 1\nmodels:\n  embedding: none@v0\n  extraction: none@v0\n  composer: none@v0\nindex:\n  engine: sqlite\nfamilies: {}\nconnectors:\n  shell:\n    command: id\n"
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load accepted an unknown connector kind; want an error naming it")
	}
	if !strings.Contains(err.Error(), "unknown key") || !strings.Contains(err.Error(), "shell") {
		t.Fatalf("Load error = %q, want it to contain \"unknown key\" and name \"shell\"", err)
	}
}

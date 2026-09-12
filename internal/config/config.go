// Package config loads and writes serenity.yml: connectors, the pinned
// model set (RFC §7.5), the index engine, and the predicate-family
// vocabulary with storage tiers (§7.2, §7.2a). The vocabulary is
// extensible only via this file plus migration — never ad hoc by workers.
package config

import (
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/ladder"
)

// FileName is the canonical config file name at the brain repo root.
const FileName = "serenity.yml"

// Models is the pinned model set (§7.5): exact provider+model+version
// identifiers. The byte-identical rebuild invariant is asserted only
// within an unchanged pinned set; changing a pin is a migration.
type Models struct {
	Embedding  string `yaml:"embedding"`
	Extraction string `yaml:"extraction"`
	// Composer is the judgment-tier model `serenity ask` (T1.12, RFC
	// §11) routes TaskClassComposerSynthesis calls through. Same
	// "<model>@<version>" / "none@v0" convention as the other two pins.
	Composer string `yaml:"composer"`
	// Provider explicitly selects which adapter internal/providers'
	// Build*Router functions build for the Extraction and Composer pins:
	// "openrouter" | "anthropic" | "openai" (ADR 013). Empty -- the zero
	// value for every brain created before ADR 013 -- falls back to the
	// pre-existing "claude" substring inference on the model name, so
	// existing brains are unaffected. BuildEmbeddingRouter never reads
	// this field: OpenRouter has no embeddings endpoint.
	Provider string `yaml:"provider,omitempty"`
	// DisableThinking, when true, asks an OpenAI-compatible chat endpoint
	// to skip a reasoning-capable model's default "thinking" pass
	// (SGLang/vLLM's chat_template_kwargs.enable_thinking, sent verbatim
	// via router.OpenAICompatibleProvider.ExtraBody) for the Extraction
	// and Composer pins (T1.31; docs/devlog.md 2026-09-06 measures a
	// 6-15x per-call wall-clock reduction against a live qwen3.8-27b
	// endpoint, with equivalent extraction quality on the sampled
	// chunks). Default false so existing brains and real OpenAI/
	// OpenRouter calls are unaffected -- a real OpenAI/OpenRouter
	// endpoint does not recognize this field and some reject unknown
	// top-level fields outright, so this must stay opt-in, never a
	// default. BuildEmbeddingRouter never reads this field.
	DisableThinking bool `yaml:"disable_thinking,omitempty"`
}

// Family declares one predicate family: its storage tier and the
// confidence-decay half-life used by ranking (§10.2).
type Family struct {
	Tier         domain.Tier `yaml:"tier"`
	HalfLifeDays int         `yaml:"half_life_days"`
}

// Index selects the derived-index engine. SQLite is the default;
// Postgres+pgvector is the documented scale profile (§7.5).
type Index struct {
	Engine string `yaml:"engine"`
}

// Server configures the daemon's HTTP transport (RFC §14: "binds
// localhost with a bearer token required by default; LAN/Tailscale
// exposure is explicit config with token + optional mTLS"). The zero
// value is the secure default: loopback only, no mTLS.
type Server struct {
	// Bind is the listen address ("host:port"). Empty selects the
	// transport's own loopback default.
	Bind string `yaml:"bind,omitempty"`
	// AllowLAN is the explicit, separately-named opt-in RFC §14 requires
	// before Bind may resolve to anything but a loopback address.
	AllowLAN bool `yaml:"allow_lan,omitempty"`
	// ClientCAFile, set together with ServerCertFile/ServerKeyFile, turns
	// on mTLS: connections must present a certificate signed by this CA.
	// Only meaningful when AllowLAN is true.
	ClientCAFile string `yaml:"client_ca_file,omitempty"`
	// ServerCertFile and ServerKeyFile are the daemon's own TLS identity,
	// required alongside ClientCAFile for mTLS.
	ServerCertFile string `yaml:"server_cert_file,omitempty"`
	ServerKeyFile  string `yaml:"server_key_file,omitempty"`
	// MaxInFlightCalls bounds concurrent MCP tool calls for this daemon's
	// HTTP handler. Zero uses the transport default.
	MaxInFlightCalls int `yaml:"max_in_flight_calls,omitempty"`
}

type Config struct {
	Version    int               `yaml:"version"`
	Models     Models            `yaml:"models"`
	Index      Index             `yaml:"index"`
	Server     Server            `yaml:"server,omitempty"`
	Families   map[string]Family `yaml:"families"`
	Connectors map[string]any    `yaml:"connectors,omitempty"`
	// Ladder is the earned-automation ladder policy object (RFC §10.3,
	// T2.10). config.Default seeds the RFC's published priors; T2.11's
	// calibration sweep replaces them with evidence-backed defaults before
	// launch. A brain repo created before T2.10 has no "ladder:" key in its
	// serenity.yml at all, so Load leaves this at its Go zero value on
	// those brains -- every field 0/empty, which internal/ladder's mandatory
	// correlation-guard validation (ParseConfig) never runs against here,
	// since plain per-field YAML decoding (not ParseConfig) is what Load
	// uses. A zero-value Ladder is inert: T2.2's reconcile engine (not yet
	// built) is expected to treat a config whose correlation guards are
	// both 0 as "ladder not configured for this brain" rather than feeding
	// it to ladder.NewEngine, the same disclosed-scope pattern ADR 013 used
	// for the Provider field's empty-string fallback.
	Ladder ladder.Config `yaml:"ladder,omitempty"`
}

// Default returns the install-time seed: the controlled predicate
// vocabulary of §7.2 with the tier assignments of §7.2a (balances,
// costs, and transaction-shaped families are shard-tier).
func Default() *Config {
	return &Config{
		Version: 1,
		Models: Models{
			// No models pinned at init. Extraction and embedding are
			// configured (and pinned) when the user connects a provider;
			// "none@v0" keeps the rebuild-identity assertion honest.
			Embedding:  "none@v0",
			Extraction: "none@v0",
			Composer:   "none@v0",
			// ADR 013: new brains default to OpenRouter for extraction/
			// composer once a model is pinned; this does not un-skip the
			// "no model pinned" default above.
			Provider: "openrouter",
		},
		Index: Index{Engine: "sqlite"},
		Families: map[string]Family{
			"works_at":           {Tier: domain.TierFence, HalfLifeDays: 90},
			"has_role":           {Tier: domain.TierFence, HalfLifeDays: 90},
			"owns_account":       {Tier: domain.TierFence, HalfLifeDays: 365},
			"has_balance":        {Tier: domain.TierShard, HalfLifeDays: 1},
			"has_condition":      {Tier: domain.TierFence, HalfLifeDays: 365},
			"takes_medication":   {Tier: domain.TierFence, HalfLifeDays: 90},
			"prefers":            {Tier: domain.TierFence, HalfLifeDays: 365},
			"committed_to":       {Tier: domain.TierFence, HalfLifeDays: 30},
			"deadline_on":        {Tier: domain.TierFence, HalfLifeDays: 30},
			"relates_to":         {Tier: domain.TierFence, HalfLifeDays: 365},
			"belongs_to_project": {Tier: domain.TierFence, HalfLifeDays: 180},
			"said":               {Tier: domain.TierFence, HalfLifeDays: 365},
			"costs":              {Tier: domain.TierShard, HalfLifeDays: 30},
		},
		Ladder: *ladder.DefaultConfig(),
	}
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &c, nil
}

func (c *Config) Save(path string) error {
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// TierOf returns the storage tier for a predicate family. Unknown
// families default to fence-tier (the conservative, human-readable home).
func (c *Config) TierOf(family string) domain.Tier {
	if f, ok := c.Families[family]; ok && f.Tier == domain.TierShard {
		return domain.TierShard
	}
	return domain.TierFence
}

// FamilyNames returns the vocabulary in sorted order (deterministic
// iteration for writers and reports).
func (c *Config) FamilyNames() []string {
	names := make([]string, 0, len(c.Families))
	for n := range c.Families {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

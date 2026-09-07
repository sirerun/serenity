// Package ladder implements the earned-automation ladder policy object
// (RFC 0001 §10.3): thresholds under which a (connector, predicate-family)
// cell earns the right to auto-resolve conflicts, instead of every
// conflict landing in the DISPOSITION queue (Trust 0, the default and
// gbrain's own posture).
//
// The policy is configurable, not a set of hardcoded constants — sparse
// cells and dense cells need different thresholds, and promotion also
// requires "correlation guards" (a minimum time span and source-diversity
// count) so a single bad connector day or one labeling mood cannot alone
// promote a cell. Every automatic action a Trust-1 cell takes is still
// logged as an auto disposition, sampled into the human audit queue at
// the cell's configured rate. A human reversal of any auto action
// unconditionally demotes the cell back to Trust 0.
//
// This package is deliberately decoupled from internal/disposition (a
// concurrent sibling task, T2.1, not a dependency of this one — see
// docs/plan.md deps: [T0.10]): it defines its own minimal Recorder
// interface for auto-action logging rather than importing that package's
// concrete store, so promotion/demotion/logging can be built, tested, and
// shipped independently of the disposition store's own schedule. Whatever
// store T2.1 ships adapts to Recorder when the reconcile engine (T2.2)
// wires the two together.
package ladder

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"gopkg.in/yaml.v3"
)

// CellPolicy is one (connector, predicate-family) cell's automation-earning
// thresholds (RFC §10.3).
type CellPolicy struct {
	// MinDispositions is the minimum disposition-history count before a
	// cell is even considered for promotion.
	MinDispositions int `yaml:"min_dispositions"`
	// MinAccept is the minimum accept rate (RFC: "the promotion metric
	// being false-acceptance rate on sampled audits, not raw accept rate"
	// — that sampled-audit refinement is T2.11's calibration scope; this
	// package's Evaluate uses the disposition-history accept rate its
	// caller supplies, which the calibration sweep is expected to source
	// from sampled audits once it exists).
	MinAccept float64 `yaml:"min_accept"`
	// SampleRate is the fraction of this cell's auto actions additionally
	// raised into the human audit queue (every auto action is logged
	// regardless — see Recorder).
	SampleRate float64 `yaml:"sample_rate"`
}

// CorrelationGuards are mandatory promotion guards against correlated
// error: RFC §10.3 — "one bad connector day or one labeling mood cannot
// promote a cell" — so both fields are required in every ladder config;
// ParseConfig refuses a document that omits either.
type CorrelationGuards struct {
	MinSpanDays        int `yaml:"min_span_days"`
	MinDistinctSources int `yaml:"min_distinct_sources"`
}

// Config is the ladder policy object (RFC §10.3): a configurable
// promotion policy, not hardcoded constants.
type Config struct {
	Default           CellPolicy            `yaml:"default"`
	PerCell           map[string]CellPolicy `yaml:"per_cell,omitempty"`
	CorrelationGuards CorrelationGuards     `yaml:"correlation_guards"`
	// NeverAutomate lists cell keys that must never promote past Trust 0
	// regardless of disposition history — RFC's seed list covers kinds of
	// dispositions structurally unsafe to automate (precept-touching
	// conflicts, effect requests, tombstones) plus the one action-set
	// entry (contact_new_party) that is equally unsafe to auto-approve.
	NeverAutomate []string `yaml:"never_automate,omitempty"`
}

// DefaultConfig returns the RFC §10.3 priors: shipped install-time values,
// explicitly documented as priors to be replaced by evidence before launch
// (T2.11's calibration sweep, RFC: "the numbers above are priors, replaced
// by evidence before launch").
func DefaultConfig() *Config {
	return &Config{
		Default: CellPolicy{MinDispositions: 50, MinAccept: 0.98, SampleRate: 0.05},
		PerCell: map[string]CellPolicy{
			"email/has_balance": {MinDispositions: 50, MinAccept: 0.99, SampleRate: 0.05},
			"email/works_at":    {MinDispositions: 20, MinAccept: 0.95, SampleRate: 0.10},
		},
		CorrelationGuards: CorrelationGuards{MinSpanDays: 14, MinDistinctSources: 5},
		NeverAutomate:     []string{"precept-touching", "effect-requests", "tombstones", "contact_new_party"},
	}
}

// PolicyFor returns cell's policy, falling back to Default when no
// per-cell override exists.
func (c *Config) PolicyFor(cell string) CellPolicy {
	if p, ok := c.PerCell[cell]; ok {
		return p
	}
	return c.Default
}

// NeverAutomates reports whether cell can never promote past Trust 0,
// regardless of its disposition history.
func (c *Config) NeverAutomates(cell string) bool {
	for _, n := range c.NeverAutomate {
		if n == cell {
			return true
		}
	}
	return false
}

// ParseConfig decodes and validates a ladder policy document — the
// "ladder:" section of serenity.yml, or a standalone document with the
// same shape (both forms parse identically: this function is handed the
// bytes of the ladder mapping itself, e.g. RFC §10.3's own example
// block). CorrelationGuards' two fields are mandatory: a document that
// omits either min_span_days or min_distinct_sources — including one that
// omits the whole correlation_guards block — fails to parse rather than
// silently defaulting to 0, which would make every cell trivially pass
// both guards on its very first disposition.
func ParseConfig(data []byte) (*Config, error) {
	var probe struct {
		CorrelationGuards yaml.Node `yaml:"correlation_guards"`
	}
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("ladder: parse config: %w", err)
	}
	if !hasMappingKey(&probe.CorrelationGuards, "min_span_days") {
		return nil, fmt.Errorf("ladder: parse config: correlation_guards.min_span_days is required")
	}
	if !hasMappingKey(&probe.CorrelationGuards, "min_distinct_sources") {
		return nil, fmt.Errorf("ladder: parse config: correlation_guards.min_distinct_sources is required")
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("ladder: parse config: %w", err)
	}
	return &cfg, nil
}

// hasMappingKey reports whether node is a YAML mapping node with the
// given scalar key present. A zero-value node (the key was absent from
// the parent document entirely) is never a MappingNode, so this also
// covers "correlation_guards" being missing outright, not just one of its
// two subfields.
func hasMappingKey(node *yaml.Node, key string) bool {
	if node == nil || node.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return true
		}
	}
	return false
}

// Trust is a cell's earned-automation level (RFC §10.3).
type Trust int

const (
	// Trust0 is the default and starting state: every conflict is a
	// disposition item (gbrain's posture).
	Trust0 Trust = iota
	// Trust1 permits auto-resolution for this cell; every auto action is
	// still logged and sampled (see Recorder).
	Trust1
)

func (t Trust) String() string {
	if t == Trust1 {
		return "trust1"
	}
	return "trust0"
}

// Disposition is the minimal shape Evaluate needs from a cell's
// disposition history: whether the item was accepted, when it was
// disposed, and which source contributed it (for the distinct-source
// correlation guard). Deliberately narrower than internal/disposition's
// eventual item shape — see the package doc.
type Disposition struct {
	Accepted   bool
	OccurredAt time.Time
	SourceID   string
}

// AutoAction is one automatic action a Trust-1 cell just took.
type AutoAction struct {
	Cell      string
	Timestamp time.Time
	// Sampled reports whether this action was additionally selected for
	// human audit at the cell's configured sample_rate. RFC §10.3: "every
	// automatic action is still logged as a disposition item marked auto
	// and sampled into the queue at the cell's rate" — Sampled is that
	// selection, not a gate on whether the action is logged at all: every
	// AutoAction reaches Recorder regardless of this field.
	Sampled bool
}

// Recorder persists one auto-action log entry. internal/ladder defines
// this narrow interface rather than depending on a concrete disposition
// store (see package doc); whatever store lands adapts to it.
type Recorder interface {
	RecordAutoAction(ctx context.Context, action AutoAction) error
}

// Engine tracks per-cell trust state in memory for the lifetime of one
// process. It does not persist state itself — callers (the reconcile
// engine, T2.2) are responsible for durable storage of promotion/
// demotion transitions if a restart must not forget them; nothing in
// RFC §10.3 requires that durability of this package specifically.
type Engine struct {
	cfg   *Config
	state map[string]Trust

	// RandFloat64, when set, replaces math/rand's Float64 for sampling —
	// inject a deterministic stub in tests. Defaults to rand.Float64.
	RandFloat64 func() float64
}

// NewEngine constructs an Engine with every cell starting at Trust0.
func NewEngine(cfg *Config) *Engine {
	return &Engine{cfg: cfg, state: make(map[string]Trust)}
}

// TrustOf reports cell's current trust state (Trust0 for any cell never
// promoted).
func (e *Engine) TrustOf(cell string) Trust {
	return e.state[cell]
}

// Evaluate reports whether cell's full disposition history satisfies its
// policy's count/accept-rate thresholds AND the config's mandatory
// correlation guards (time span, distinct sources), and that cell is not
// listed in never_automate. It does not mutate Engine state.
func (e *Engine) Evaluate(cell string, history []Disposition) bool {
	if e.cfg.NeverAutomates(cell) {
		return false
	}
	policy := e.cfg.PolicyFor(cell)
	if len(history) < policy.MinDispositions {
		return false
	}

	accepted := 0
	sources := make(map[string]bool, len(history))
	var minAt, maxAt time.Time
	for i, d := range history {
		if d.Accepted {
			accepted++
		}
		sources[d.SourceID] = true
		if i == 0 || d.OccurredAt.Before(minAt) {
			minAt = d.OccurredAt
		}
		if i == 0 || d.OccurredAt.After(maxAt) {
			maxAt = d.OccurredAt
		}
	}

	acceptRate := float64(accepted) / float64(len(history))
	if acceptRate < policy.MinAccept {
		return false
	}

	spanDays := int(maxAt.Sub(minAt).Hours() / 24)
	if spanDays < e.cfg.CorrelationGuards.MinSpanDays {
		return false
	}
	if len(sources) < e.cfg.CorrelationGuards.MinDistinctSources {
		return false
	}

	return true
}

// Promote evaluates cell's history and, if the policy and correlation
// guards are satisfied, sets its trust state to Trust1. Reports whether
// promotion happened.
func (e *Engine) Promote(cell string, history []Disposition) bool {
	if !e.Evaluate(cell, history) {
		return false
	}
	e.state[cell] = Trust1
	return true
}

// Demote unconditionally drops cell back to Trust0. RFC §10.3: "Demotion
// is automatic: a human reversal of an auto action drops the cell back to
// Trust 0" — never gated by policy, unlike Promote.
func (e *Engine) Demote(cell string) {
	e.state[cell] = Trust0
}

// LogAutoAction records one automatic action for cell via rec,
// unconditionally, and marks it Sampled per the cell's configured
// sample_rate so a fraction of auto actions are additionally raised for
// human audit.
func (e *Engine) LogAutoAction(ctx context.Context, rec Recorder, cell string, at time.Time) error {
	policy := e.cfg.PolicyFor(cell)
	randFloat64 := e.RandFloat64
	if randFloat64 == nil {
		randFloat64 = rand.Float64
	}
	action := AutoAction{
		Cell:      cell,
		Timestamp: at,
		Sampled:   randFloat64() < policy.SampleRate,
	}
	return rec.RecordAutoAction(ctx, action)
}

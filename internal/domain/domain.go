// Package domain defines the epistemic layers contract (RFC 0001 §7.6):
// Source -> Observation -> Claim -> Precept. Each layer has distinct
// authority; nothing here writes to disk — that is the store's job, and
// every canonical write goes through the deterministic writers.
package domain

import "time"

// Visibility is present in v1 file formats and ignored by the daemon
// (RFC §6: the format must not preclude multi-principal later).
type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityShared  Visibility = "shared"
)

// State is the claim lifecycle state; supersession keeps the row (§7.2).
type State string

const (
	StateActive     State = "active"
	StateSuperseded State = "superseded"
	StateRetracted  State = "retracted"
)

// Tier is a predicate family's storage tier (§7.2a).
type Tier string

const (
	TierFence Tier = "fence"
	TierShard Tier = "shard"
)

// Provenance points at the observation-bearing source span with the model
// pin that produced it (§7.5). Actor is "machine" or "human:<id>".
type Provenance struct {
	SourceSHA256 string    `json:"source_sha256,omitempty"`
	Span         string    `json:"span,omitempty"`
	Model        string    `json:"model,omitempty"`
	ObservedAt   time.Time `json:"observed_at,omitzero"`
	Actor        string    `json:"actor,omitempty"`
}

// Claim is the atomic unit: subject–predicate–object with confidence,
// validity window, and supersession chain (§7.2). IDs are content-derived
// short hashes, stable across rebuilds.
type Claim struct {
	ID          string  `json:"id"`
	SubjectSlug string  `json:"subject"`
	Predicate   string  `json:"predicate"`
	Object      string  `json:"object"`
	ObjectKey   string  `json:"object_key,omitempty"`
	Confidence  float64 `json:"confidence"`
	ValidFrom   string  `json:"valid_from,omitempty"`
	ValidTo     string  `json:"valid_to,omitempty"`
	// Visibility exists in v1 and is ignored by the daemon (§6).
	Visibility Visibility `json:"visibility,omitempty"`
	State      State      `json:"state"`
	// Supersedes points at the claim this line replaces (shard chains
	// append a superseding line rather than rewriting history, §7.2a).
	Supersedes string `json:"supersedes,omitempty"`
	// SupersededBy is the forward pointer rendered in fence rows (§7.2).
	SupersededBy string `json:"superseded_by,omitempty"`
	// SourceRef is the human-readable src cell (e.g. "e42#3", "shard").
	SourceRef  string     `json:"src,omitempty"`
	Family     string     `json:"family"`
	Provenance Provenance `json:"provenance,omitzero"`
}

// Observation is an immutable machine extraction tied to one source span —
// not yet believed (epistemic layer 2, §7.6).
type Observation struct {
	ID           string    `json:"id"`
	SubjectSlug  string    `json:"subject"`
	Predicate    string    `json:"predicate"`
	Object       string    `json:"object"`
	Confidence   float64   `json:"confidence"`
	Model        string    `json:"model"`
	SourceSHA256 string    `json:"source_sha256"`
	Span         string    `json:"span"`
	CreatedAt    time.Time `json:"created_at"`
}

// Source is raw imported material: content-addressed, immutable (layer 1).
type Source struct {
	SHA256     string            `json:"sha256"`
	Kind       string            `json:"kind"` // email | file | git_repo | voice
	URI        string            `json:"uri"`
	OccurredAt time.Time         `json:"occurred_at"`
	IndexOnly  bool              `json:"index_only,omitempty"`
	Meta       map[string]string `json:"meta,omitempty"`
}

// Entity is the join key: person, company, account, project, topic, or
// health item — one markdown page per entity (§7.2).
type Entity struct {
	Type    string   `json:"type"`
	Slug    string   `json:"slug"`
	Aliases []string `json:"aliases,omitempty"`
}

// PreceptKind mirrors dira's entry kinds (§7.3).
type PreceptKind string

const (
	PreceptDecision   PreceptKind = "decision"
	PreceptConstraint PreceptKind = "constraint"
	PreceptIntent     PreceptKind = "intent"
	PreceptQuestion   PreceptKind = "question"
)

// ActionSet is the closed action set constraints apply to (§7.3).
var ActionSet = []string{
	"start_project",
	"deploy_to_prod",
	"spend_over",
	"contact_new_party",
	"schedule_outside_hours",
}

// AppliesWhen guards a constraint against structured actions (§8.3).
type AppliesWhen struct {
	Action string         `json:"action"`
	Params map[string]any `json:"params,omitempty"`
}

// RejectedAlternative records why_not / revisit_if — dira schema fields.
type RejectedAlternative struct {
	Option    string `json:"option"`
	WhyNot    string `json:"why_not"`
	RevisitIf string `json:"revisit_if,omitempty"`
}

// Precept is an immutable record of human judgment: supersede-only, never
// edited, creatable only through human disposition (§7.3 security
// invariant). Confidence never applies to precepts (§7.6).
type Precept struct {
	ID           string                `json:"id"`
	Kind         PreceptKind           `json:"kind"`
	Title        string                `json:"title"`
	Body         string                `json:"body"`
	AppliesWhen  *AppliesWhen          `json:"applies_when,omitempty"`
	Rejected     []RejectedAlternative `json:"rejected,omitempty"`
	Supersedes   string                `json:"supersedes,omitempty"`
	SupersededBy string                `json:"superseded_by,omitempty"`
	Author       string                `json:"author"`
	CreatedAt    time.Time             `json:"created_at"`
}

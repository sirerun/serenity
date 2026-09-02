// Package serenity is the read-only embedding facade over a Serenity brain
// repo, for consumers that run Serenity's read paths in-process instead of
// over the wire (ADR 012, docs/adr/012-embedded-read-facade-single-writer.md).
//
// The facade adds no logic. Each method calls exactly the internal function
// the corresponding CLI verb calls, with the same arguments, and a drift
// test (drift_test.go) deep-equals CheckPlan against `serenity check --json
// --actions` so the two surfaces cannot diverge silently (RFC 0001 §13.1:
// no privileged internal path). Every handle is read-only: Open builds its
// ledger with no writer queue, so every ledger mutator errors
// (direction.ErrReadOnly) rather than writing, and no writer entry point is
// exported -- writes to a brain stay with the one brain-writer process
// (the Serenity daemon or CLI, ADR 004), reached over the wire.
//
// This package is a consumer surface bound by the protocol_version policy
// of ADR 012 §5 and RFC 0001 §8: field names and semantics are frozen once
// shipped, changes are additive and optional, and a breaking change is a
// new package path. Brief lands in T4.19 (docs/plans/E4-m4-serve-protocols.md).
package serenity

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/sirerun/serenity/internal/compose"
	"github.com/sirerun/serenity/internal/config"
	"github.com/sirerun/serenity/internal/direction"
	"github.com/sirerun/serenity/internal/direction/check"
	"github.com/sirerun/serenity/internal/embed"
	"github.com/sirerun/serenity/internal/providers"
	"github.com/sirerun/serenity/internal/search"
)

// Option configures Open. No Option values ship at T4.18; the parameter
// exists so ADR 012 §1's Open signature stays stable when one is added
// (additive-optional, RFC 0001 §8).
type Option func(*Brain) error

// Brain is a read-only handle on one brain repo. It holds no open files
// between calls: like the CLI verbs it mirrors, each method opens what it
// needs and releases it before returning, so a Brain needs no Close.
type Brain struct {
	root  string
	cfg   *config.Config
	store *direction.Store
}

// Open returns a read-only Brain over the brain repo at brainPath. It fails
// when brainPath carries no serenity.yml, the same check every CLI verb
// makes first. The ledger handle it builds has no writer queue
// (direction.NewStore(root, nil)), which is what makes the handle read-only
// by construction rather than by promise.
func Open(brainPath string, opts ...Option) (*Brain, error) {
	cfg, err := config.Load(filepath.Join(brainPath, config.FileName))
	if err != nil {
		return nil, fmt.Errorf("serenity: open %s: not a brain repo: %w", brainPath, err)
	}
	b := &Brain{root: brainPath, cfg: cfg, store: direction.NewStore(brainPath, nil)}
	for _, opt := range opts {
		if err := opt(b); err != nil {
			return nil, fmt.Errorf("serenity: open %s: %w", brainPath, err)
		}
	}
	return b, nil
}

// Action is one structured plan action (RFC 0001 §8.3 `actions[]`): a
// member of the closed action set plus its parameters, e.g.
// {Action: "spend_over", Params: {"amount": 500}}. Its JSON form is one
// element of `serenity check --actions`'s array.
type Action struct {
	Action string         `json:"action"`
	Params map[string]any `json:"params,omitempty"`
}

// Verdict is CheckPlan's result: byte-for-byte the object `serenity check
// --json --actions` prints (internal/cli/check.go's JSON shape, RFC 0001
// §8.3 vocabulary). Status is "pass", "violated" or
// "no_applicable_constraints" -- never a bare pass for a plan that matched
// nothing (ADR 010).
type Verdict struct {
	Status          string       `json:"status"`
	ConsideredCount int          `json:"considered_count"`
	Constraints     []Constraint `json:"constraints,omitempty"`
	Warnings        []Warning    `json:"warnings,omitempty"`
}

// Constraint is one applicable active constraint's verdict. WhyNot and
// RevisitIf are populated only when Outcome is "violated", copied verbatim
// from the precept's first recorded alternative.
type Constraint struct {
	PreceptID string `json:"precept_id"`
	Outcome   string `json:"outcome"`
	WhyNot    string `json:"why_not,omitempty"`
	RevisitIf string `json:"revisit_if,omitempty"`
}

// Warning is one open question precept that blocks a checked action. A
// warning never changes Verdict.Status (RFC 0001 §8.3).
type Warning struct {
	PreceptID string `json:"precept_id"`
	Title     string `json:"title"`
	Action    string `json:"action"`
}

// CheckPlan checks actions against every active constraint precept in the
// brain's ledger, exactly as `serenity check --json --actions` does:
// check.New over the read-only ledger handle with no classification
// router, then Matcher.Match. An action outside the closed action set is
// an error (check.ErrUnknownAction), not a verdict.
func (b *Brain) CheckPlan(ctx context.Context, actions []Action) (Verdict, error) {
	in := make([]check.Action, len(actions))
	for i, a := range actions {
		in[i] = check.Action{Action: a.Action, Params: a.Params}
	}
	result, err := check.New(b.store, nil).Match(ctx, in)
	if err != nil {
		return Verdict{}, fmt.Errorf("serenity: check plan: %w", err)
	}
	v := Verdict{Status: string(result.Status), ConsideredCount: result.ConsideredCount}
	for _, c := range result.Constraints {
		v.Constraints = append(v.Constraints, Constraint{
			PreceptID: c.PreceptID, Outcome: string(c.Outcome), WhyNot: c.WhyNot, RevisitIf: c.RevisitIf,
		})
	}
	for _, w := range result.Warnings {
		v.Warnings = append(v.Warnings, Warning{PreceptID: w.PreceptID, Title: w.Title, Action: w.Action})
	}
	return v, nil
}

// defaultMaxResults is `serenity search`'s --limit default.
const defaultMaxResults = 10

// Budget bounds one Recall. MaxResults caps the ranked hits (the
// `serenity search --limit` value); zero means the CLI default, 10.
type Budget struct {
	MaxResults int `json:"max_results"`
}

// Hit is one ranked, deduplicated search result: internal/search.Result's
// fields, with Score the fused RRF score.
type Hit struct {
	ChunkRef     string  `json:"chunk_ref"`
	EntitySlug   string  `json:"entity_slug"`
	Kind         string  `json:"kind"`
	Text         string  `json:"text"`
	SourceSHA256 string  `json:"source_sha256"`
	Score        float64 `json:"score"`
}

// Citation is one claim an Answer used to ground a fact (RFC 0001 §11).
type Citation struct {
	ClaimID    string    `json:"claim_id"`
	Subject    string    `json:"subject"`
	Predicate  string    `json:"predicate"`
	Object     string    `json:"object"`
	SourceRef  string    `json:"source_ref"`
	Confidence float64   `json:"confidence"`
	ObservedAt time.Time `json:"observed_at"`
	ValidTo    string    `json:"valid_to,omitempty"`
}

// Supersession is one cited fact's full lifecycle, oldest first, ending at
// the currently-live claim.
type Supersession struct {
	Subject   string     `json:"subject"`
	Predicate string     `json:"predicate"`
	Chain     []Citation `json:"chain"`
}

// Answer is the composer's cited answer (RFC 0001 §11). Text and Gap are
// mutually exclusive: Gap is set exactly when no claim answers the
// question, naming the newest evidence's age.
type Answer struct {
	Text          string         `json:"text,omitempty"`
	Citations     []Citation     `json:"citations,omitempty"`
	Supersessions []Supersession `json:"supersessions,omitempty"`
	Gap           string         `json:"gap,omitempty"`
}

// RecallResult is what Recall returns: the ranked hits `serenity search`
// prints and, when a composer model is pinned and credentialed, the cited
// answer `serenity ask` prints. Answer is nil exactly when the composer is
// unavailable, and Note then carries the CLI's own one-line reason (the
// same text `serenity ask` prints instead of an answer).
type RecallResult struct {
	Hits   []Hit   `json:"hits,omitempty"`
	Answer *Answer `json:"answer,omitempty"`
	Note   string  `json:"note,omitempty"`
}

// Recall runs the two read paths `serenity search` and `serenity ask` run,
// in that order, over the brain's derived index: search.Search with no
// embedder (the CLI's full-text-only search wiring, internal/cli/search.go),
// then compose.New + Composer.Ask with the composer router and query
// embedder built from serenity.yml exactly as internal/cli/ask.go builds
// them. Ask is a judgment-tier model call and records spend through the
// index's spend ledger, as the CLI verb does.
func (b *Brain) Recall(ctx context.Context, q string, budget Budget) (RecallResult, error) {
	limit := budget.MaxResults
	if limit == 0 {
		limit = defaultMaxResults
	}
	eng, err := providers.OpenIndex(b.root)
	if err != nil {
		return RecallResult{}, fmt.Errorf("serenity: recall: open index: %w", err)
	}
	defer func() { _ = eng.Close() }()

	results, err := search.Search(ctx, eng, nil, q, limit, search.Options{})
	if err != nil {
		return RecallResult{}, fmt.Errorf("serenity: recall: %w", err)
	}
	out := RecallResult{}
	for _, r := range results {
		out.Hits = append(out.Hits, Hit{
			ChunkRef: r.ChunkRef, EntitySlug: r.EntitySlug, Kind: r.Kind, Text: r.Text,
			SourceSHA256: r.SourceSHA256, Score: r.RRFScore,
		})
	}

	ledger := &providers.IndexSpendLedger{Eng: eng}
	r, ok, note := providers.BuildComposerRouter(b.cfg, ledger)
	if !ok {
		out.Note = note
		return out, nil
	}
	var embedder embed.Embedder
	if er, eok, enote := providers.BuildEmbeddingRouter(b.cfg, ledger); eok {
		embedder = &embed.RouterEmbedder{Router: er, Pin: b.cfg.Models.Embedding}
	} else {
		out.Note = enote + " -- widening query relevance to full-text/lexical matching only"
	}
	answer, err := compose.New(b.root, b.cfg, eng, embedder, r, b.cfg.Models.Composer).Ask(ctx, q)
	if err != nil {
		return RecallResult{}, fmt.Errorf("serenity: recall: ask: %w", err)
	}
	out.Answer = toAnswer(answer)
	return out, nil
}

func toAnswer(a compose.Answer) *Answer {
	out := &Answer{Text: a.Text, Gap: a.Gap}
	for _, c := range a.Citations {
		out.Citations = append(out.Citations, toCitation(c))
	}
	for _, s := range a.Supersessions {
		sup := Supersession{Subject: s.Subject, Predicate: s.Predicate}
		for _, c := range s.Chain {
			sup.Chain = append(sup.Chain, toCitation(c))
		}
		out.Supersessions = append(out.Supersessions, sup)
	}
	return out
}

func toCitation(c compose.Citation) Citation {
	return Citation{
		ClaimID: c.ClaimID, Subject: c.Subject, Predicate: c.Predicate, Object: c.Object,
		SourceRef: c.SourceRef, Confidence: c.Confidence, ObservedAt: c.ObservedAt, ValidTo: c.ValidTo,
	}
}

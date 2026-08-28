// Package brainbench adapts dndungu/gbrain's BrainBench corpus (vendored
// verbatim at evals/brainbench, pinned in evals/brainbench/PIN, T1.21) onto
// Serenity's own hybrid search (internal/search, T1.11) so CI can track
// retrieval quality against an external, independently-authored benchmark
// rather than only Serenity's own golden tests.
//
// Scope (disclosed here once; Evaluate's doc comment repeats the summary):
// BrainBench's full harness measures four things an autonomous memory
// agent does -- decide when to retrieve unprompted (know-to-ask), decide
// what to inject when it does (push), persist conversational facts
// (write-back), and recall a decision across harness sessions
// (continuity). Serenity has none of the autonomous-decision or
// write-back machinery that requires (T1.9 write path, T1.12 composer);
// what exists is `serenity search <query>`, invoked on demand. This
// package scores the one thing that comparison is actually honest about:
// for every gold turn marked should_retrieve:true, does Serenity's search
// ranking surface the right page(s)? Turns marked should_retrieve:false,
// and fixtures whose content depends on machinery Serenity doesn't have
// yet (write-back fixtures, continuity-reader fixtures with no seed pages
// of their own), are recorded as skipped rather than silently scored.
package brainbench

// SeedPage is one page BrainBench seeds into the fixture's brain before
// the conversation turns run. Slug is the page's stable identity (what
// gold_slugs/acceptable_slugs reference); SourceID marks a page as
// belonging to a non-default source in multi-source fixtures (unused by
// this adapter -- see Evaluate's doc comment on multi-source scope).
type SeedPage struct {
	Slug     string `json:"slug"`
	Content  string `json:"content"`
	SourceID string `json:"source_id,omitempty"`
}

// SeedFact is a fact BrainBench seeds directly (bypassing extraction) --
// used by write-back/continuity fixtures this adapter does not score
// (see package doc).
type SeedFact struct {
	Fact          string  `json:"fact"`
	EntitySlug    *string `json:"entity_slug,omitempty"`
	Source        string  `json:"source,omitempty"`
	SourceSession *string `json:"source_session,omitempty"`
	SourceID      string  `json:"source_id,omitempty"`
}

// Turn is one adapter-visible conversation turn (schema/fixture.schema.json).
type Turn struct {
	TurnID int    `json:"turn_id"`
	Role   string `json:"role"`
	Text   string `json:"text"`
	TS     string `json:"ts,omitempty"`
}

// Continuity marks a fixture as one half of a writer/reader pair.
type Continuity struct {
	PairID   string `json:"pair_id"`
	PairRole string `json:"pair_role"`
}

// Fixture is one BrainBench *.fixture.json (schema_version 1). Gold
// annotations are sealed -- never present here, always joined from a
// separate Gold record by FixtureID.
type Fixture struct {
	SchemaVersion int         `json:"schema_version"`
	FixtureID     string      `json:"fixture_id"`
	Suites        []string    `json:"suites"`
	Category      string      `json:"category,omitempty"`
	Holdout       bool        `json:"holdout,omitempty"`
	Sources       []string    `json:"sources,omitempty"`
	ActiveSource  string      `json:"active_source,omitempty"`
	SeedPages     []SeedPage  `json:"seed_pages,omitempty"`
	SeedFacts     []SeedFact  `json:"seed_facts,omitempty"`
	Turns         []Turn      `json:"turns"`
	Continuity    *Continuity `json:"continuity,omitempty"`
}

// GoldFact is one write-back gold fact this adapter does not score (see
// package doc) but preserves when loading gold files.
type GoldFact struct {
	Gist          string   `json:"gist"`
	Fact          string   `json:"fact"`
	EntitySlug    *string  `json:"entity_slug"`
	MatchKeywords []string `json:"match_keywords"`
	Kind          string   `json:"kind,omitempty"`
}

// GoldTurn is one turn's sealed gold annotation. GoldSlugs are pages the
// benchmark requires be retrieved; AcceptableSlugs are pages that are fine
// to retrieve but not required (BrainBench README: "injected = fine,
// missed = no penalty").
type GoldTurn struct {
	ShouldRetrieve  bool       `json:"should_retrieve"`
	GoldSlugs       []string   `json:"gold_slugs,omitempty"`
	AcceptableSlugs []string   `json:"acceptable_slugs,omitempty"`
	GoldFacts       []GoldFact `json:"gold_facts,omitempty"`
}

// ContinuityDecision and ContinuityGold mirror the continuity block of a
// gold file -- loaded for completeness, not scored by this adapter.
type ContinuityDecision struct {
	DecisionID    string   `json:"decision_id"`
	ExpectedSlugs []string `json:"expected_slugs"`
	MatchKeywords []string `json:"match_keywords"`
}

type ContinuityGold struct {
	PairID    string               `json:"pair_id"`
	Decisions []ContinuityDecision `json:"decisions"`
}

// Gold is one BrainBench *.gold.json, joined to its Fixture by FixtureID.
// Turns is keyed by String(turn_id); a turn absent from this map carries
// no gold at all (neither scoreable nor skip-worthy -- it is simply not
// part of the benchmark).
type Gold struct {
	FixtureID  string              `json:"fixture_id"`
	Turns      map[string]GoldTurn `json:"turns"`
	Continuity *ContinuityGold     `json:"continuity,omitempty"`
}

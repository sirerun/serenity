package brainbench

import (
	"context"
	"testing"
)

// twoFixtureCorpus is a small, hand-computable fixture set: enough to pin
// exact TP/FP/FN counts by hand, the same style T1.11's own golden ranking
// test (internal/search/search_test.go) uses.
//
// Fixture "f1" (category "kta-pos"): one scoreable turn, one gold_slug
// ("people/alice") and one acceptable_slug ("companies/acme"). Its pages
// are worded so the turn's own words appear only in Alice's page and
// Acme's page, not in the unrelated "people/bob" page.
//
// Fixture "f2" (category "push", no seed_pages): entirely skipped --
// exercises the "no seed_pages" skip path.
func twoFixtureCorpus() ([]Fixture, map[string]Gold) {
	fixtures := []Fixture{
		{
			FixtureID: "f1",
			Category:  "kta-pos",
			SeedPages: []SeedPage{
				{Slug: "people/alice", Content: "Alice runs Acme and raised a pricing concern."},
				{Slug: "companies/acme", Content: "Acme is a startup that Alice founded."},
				{Slug: "people/bob", Content: "Bob is an unrelated angel investor."},
			},
			Turns: []Turn{
				{TurnID: 1, Role: "user", Text: "What did Alice say about Acme pricing?"},
				{TurnID: 2, Role: "user", Text: "Thanks, that's all for now."},
			},
		},
		{
			FixtureID: "f2",
			Category:  "push",
			Turns: []Turn{
				{TurnID: 1, Role: "user", Text: "Just got off a call."},
			},
		},
	}
	gold := map[string]Gold{
		"f1": {
			FixtureID: "f1",
			Turns: map[string]GoldTurn{
				"1": {ShouldRetrieve: true, GoldSlugs: []string{"people/alice"}, AcceptableSlugs: []string{"companies/acme"}},
				"2": {ShouldRetrieve: false},
			},
		},
		"f2": {
			FixtureID: "f2",
			Turns: map[string]GoldTurn{
				"1": {ShouldRetrieve: false},
			},
		},
	}
	return fixtures, gold
}

func TestEvaluateScoresRetrievalAgainstSealedGold(t *testing.T) {
	fixtures, gold := twoFixtureCorpus()
	report, err := Evaluate(context.Background(), fixtures, gold, 10)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if report.FixturesTotal != 2 {
		t.Fatalf("FixturesTotal = %d, want 2", report.FixturesTotal)
	}
	if report.FixturesScored != 1 {
		t.Fatalf("FixturesScored = %d, want 1 (f2 has no seed_pages)", report.FixturesScored)
	}
	if len(report.FixturesSkipped) != 1 || report.FixturesSkipped[0].FixtureID != "f2" {
		t.Fatalf("FixturesSkipped = %+v, want exactly f2", report.FixturesSkipped)
	}

	// f1's turn 1: query words overlap "people/alice" and "companies/acme"
	// (both gold-relevant) far more than "people/bob" (irrelevant) -- Bob's
	// page shares zero words with the query, so it must not be retrieved
	// at all, giving perfect precision and recall on this single query.
	got := report.Overall
	if got.Queries != 1 {
		t.Fatalf("Overall.Queries = %d, want 1", got.Queries)
	}
	if got.RecallHits != 1 || got.Missed != 0 {
		t.Fatalf("recall hits/missed = %d/%d, want 1/0 (people/alice must be retrieved)", got.RecallHits, got.Missed)
	}
	if got.FalsePositives != 0 {
		t.Fatalf("FalsePositives = %d, want 0 (people/bob shares no query terms)", got.FalsePositives)
	}
	if got.Recall != 1.0 {
		t.Fatalf("Recall = %v, want 1.0", got.Recall)
	}
	if got.Precision != 1.0 {
		t.Fatalf("Precision = %v, want 1.0 (all retrieved slugs are gold or acceptable)", got.Precision)
	}

	cat, ok := report.ByCategory["kta-pos"]
	if !ok {
		t.Fatal("ByCategory missing kta-pos")
	}
	if cat != got {
		t.Fatalf("single-fixture category metrics %+v should equal overall %+v", cat, got)
	}
	if _, ok := report.ByCategory["push"]; ok {
		t.Fatal("push category should not appear -- f2 (its only fixture) was skipped, contributing zero counts")
	}
}

// TestEvaluateAcceptableSlugDoesNotInflateRecall pins the deliberate
// precision/recall asymmetry: retrieving an acceptable_slug instead of a
// missing gold_slug must count toward precision but must NOT mask the
// recall miss.
func TestEvaluateAcceptableSlugDoesNotInflateRecall(t *testing.T) {
	fixtures := []Fixture{{
		FixtureID: "f1",
		SeedPages: []SeedPage{
			{Slug: "people/only-acceptable", Content: "Zephyr mentions widgetco frequently in every note."},
		},
		Turns: []Turn{{TurnID: 1, Role: "user", Text: "Zephyr widgetco"}},
	}}
	gold := map[string]Gold{"f1": {
		FixtureID: "f1",
		Turns: map[string]GoldTurn{
			// gold_slugs names a page that was never seeded -- it can
			// never be retrieved, so this turn must always miss on
			// recall even though the seeded (acceptable-only) page is a
			// clean precision hit.
			"1": {ShouldRetrieve: true, GoldSlugs: []string{"people/never-seeded"}, AcceptableSlugs: []string{"people/only-acceptable"}},
		},
	}}

	report, err := Evaluate(context.Background(), fixtures, gold, 10)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	got := report.Overall
	if got.RecallHits != 0 || got.Missed != 1 {
		t.Fatalf("recall hits/missed = %d/%d, want 0/1 (the gold slug was never seeded)", got.RecallHits, got.Missed)
	}
	if got.Recall != 0 {
		t.Fatalf("Recall = %v, want 0", got.Recall)
	}
	if got.PrecisionHits == 0 {
		t.Fatal("PrecisionHits = 0, want > 0 (the acceptable-only page shares query terms and must be retrieved)")
	}
	if got.Precision != 1.0 {
		t.Fatalf("Precision = %v, want 1.0 (the only retrievable page is acceptable, not a false positive)", got.Precision)
	}
}

func TestEvaluateSkipsFixturesWithNoGoldFile(t *testing.T) {
	fixtures := []Fixture{{FixtureID: "orphan", SeedPages: []SeedPage{{Slug: "a", Content: "x"}}, Turns: []Turn{{TurnID: 1, Text: "x"}}}}
	report, err := Evaluate(context.Background(), fixtures, map[string]Gold{}, 10)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if report.FixturesScored != 0 {
		t.Fatalf("FixturesScored = %d, want 0", report.FixturesScored)
	}
	if len(report.FixturesSkipped) != 1 || report.FixturesSkipped[0].Reason != "no gold file" {
		t.Fatalf("FixturesSkipped = %+v, want one entry reasoned \"no gold file\"", report.FixturesSkipped)
	}
}

// TestFTSQuerySanitizesFTS5SyntaxCharacters pins the disclosed defect
// ftsQuery's doc comment describes: a bareword "?", "(", or ")" passed
// straight into an FTS5 MATCH expression is a SQL logic error, not merely
// a no-match. Before the sanitizer existed, this exact turn text made
// search.Search return "fts5: syntax error near \"?\"" (confirmed against
// the real SQLite FTS5 engine while building this adapter). This test
// exercises the real index/search stack end to end, not just ftsQuery's
// string output, so a regression that reintroduces raw punctuation would
// fail here even if some other quoting scheme looked plausible in
// isolation.
func TestFTSQuerySanitizesFTS5SyntaxCharacters(t *testing.T) {
	fixtures := []Fixture{{
		FixtureID: "punct",
		SeedPages: []SeedPage{{Slug: "p", Content: "the deal is real and pending"}},
		Turns:     []Turn{{TurnID: 1, Text: `What's the deal (really)?`}},
	}}
	gold := map[string]Gold{"punct": {FixtureID: "punct", Turns: map[string]GoldTurn{
		"1": {ShouldRetrieve: true, GoldSlugs: []string{"p"}},
	}}}
	report, err := Evaluate(context.Background(), fixtures, gold, 10)
	if err != nil {
		t.Fatalf("Evaluate errored on a punctuation-heavy turn -- sanitizer regressed: %v", err)
	}
	if report.Overall.RecallHits != 1 {
		t.Fatalf("RecallHits = %d, want 1 (\"deal\" and \"the\" overlap the seeded page)", report.Overall.RecallHits)
	}
}

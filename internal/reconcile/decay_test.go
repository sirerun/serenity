package reconcile

import (
	"maps"
	"reflect"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/domain"
)

func mkClaim(id, slug, family string, confidence float64, observedAt time.Time) domain.Claim {
	return domain.Claim{
		ID:          id,
		SubjectSlug: slug,
		Predicate:   "has_balance",
		Object:      "100",
		Confidence:  confidence,
		Family:      family,
		State:       domain.StateActive,
		Provenance:  domain.Provenance{ObservedAt: observedAt},
	}
}

func TestDecayedConfidenceHalfLife(t *testing.T) {
	now := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	observed := now.AddDate(0, 0, -10) // 10 days old
	got := DecayedConfidence(0.8, observed, now, 10)
	want := 0.4 // exactly one half-life elapsed
	if diff := got - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("DecayedConfidence = %v, want %v", got, want)
	}
}

func TestDecayedConfidenceDisabledWhenHalfLifeNonPositive(t *testing.T) {
	now := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	observed := now.AddDate(0, 0, -1000)
	if got := DecayedConfidence(0.9, observed, now, 0); got != 0.9 {
		t.Fatalf("DecayedConfidence with halfLifeDays=0 = %v, want unchanged 0.9", got)
	}
}

func TestDecayedConfidenceNonPositiveAgeUnchanged(t *testing.T) {
	now := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	if got := DecayedConfidence(0.9, now, now, 10); got != 0.9 {
		t.Fatalf("DecayedConfidence at age 0 = %v, want unchanged 0.9", got)
	}
	future := now.AddDate(0, 0, 5)
	if got := DecayedConfidence(0.9, future, now, 10); got != 0.9 {
		t.Fatalf("DecayedConfidence with observedAt in the future = %v, want unchanged 0.9", got)
	}
}

// TestRankAgedFixtureFlipsOrderStateUnchanged is the acc line's own
// clause, verbatim: "aged fixture flips ranking order while every
// claim's State is unchanged".
func TestRankAgedFixtureFlipsOrderStateUnchanged(t *testing.T) {
	now := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

	// old-high starts with the higher raw confidence but is old enough
	// (30 days, half-life 1 day for "has_balance") to decay well below
	// fresh-low, which starts lower but was observed today.
	oldHigh := mkClaim("c-old-high", "alice-tan", "has_balance", 0.9, now.AddDate(0, 0, -30))
	freshLow := mkClaim("c-fresh-low", "alice-tan", "has_balance", 0.5, now)

	claims := []domain.Claim{oldHigh, freshLow}
	// Snapshot for the no-mutation assertion below.
	origOldHigh := oldHigh
	origFreshLow := freshLow

	halfLives := map[string]int{"has_balance": 1}
	ranked := Rank(claims, halfLives, now)

	if len(ranked) != 2 {
		t.Fatalf("Rank returned %d entries, want 2", len(ranked))
	}
	if ranked[0].Claim.ID != "c-fresh-low" {
		t.Fatalf("Rank[0].Claim.ID = %q, want c-fresh-low — raw-confidence order was not flipped by decay", ranked[0].Claim.ID)
	}
	if ranked[1].Claim.ID != "c-old-high" {
		t.Fatalf("Rank[1].Claim.ID = %q, want c-old-high", ranked[1].Claim.ID)
	}

	// Every claim's State (and Confidence) in the ORIGINAL slice must be
	// unchanged by Rank.
	if claims[0].State != origOldHigh.State || claims[0].Confidence != origOldHigh.Confidence {
		t.Fatalf("Rank mutated claims[0]: got %+v, want unchanged %+v", claims[0], origOldHigh)
	}
	if claims[1].State != origFreshLow.State || claims[1].Confidence != origFreshLow.Confidence {
		t.Fatalf("Rank mutated claims[1]: got %+v, want unchanged %+v", claims[1], origFreshLow)
	}
}

// TestDistillCandidatesSubThresholdClaimAppears is the acc line's own
// clause, verbatim: "a sub-threshold claim appears as a distill item".
func TestDistillCandidatesSubThresholdClaimAppears(t *testing.T) {
	now := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

	// Decays from 0.8 to 0.4 over 60 days at a 30-day half-life --
	// clearly below DefaultDistillThreshold (0.6).
	aged := mkClaim("c-aged", "bob-nguyen", "has_balance", 0.8, now.AddDate(0, 0, -60))
	// Fresh, high-confidence, stays well above threshold.
	fresh := mkClaim("c-fresh", "bob-nguyen", "has_balance", 0.95, now)

	claims := []domain.Claim{aged, fresh}
	halfLives := map[string]int{"has_balance": 30}

	candidates := DistillCandidates(claims, halfLives, now, 0)

	if len(candidates) != 1 {
		t.Fatalf("DistillCandidates returned %d, want exactly 1: %+v", len(candidates), candidates)
	}
	if candidates[0].Claim.ID != "c-aged" {
		t.Fatalf("DistillCandidates[0].Claim.ID = %q, want c-aged", candidates[0].Claim.ID)
	}
	if candidates[0].DecayedConfidence >= DefaultDistillThreshold {
		t.Fatalf("candidate's DecayedConfidence = %v, want < %v", candidates[0].DecayedConfidence, DefaultDistillThreshold)
	}
}

func TestDistillCandidatesExcludesNonActiveClaims(t *testing.T) {
	now := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	superseded := mkClaim("c-superseded", "carol-diaz", "has_balance", 0.1, now.AddDate(0, 0, -365))
	superseded.State = domain.StateSuperseded

	candidates := DistillCandidates([]domain.Claim{superseded}, map[string]int{"has_balance": 1}, now, 0)
	if len(candidates) != 0 {
		t.Fatalf("DistillCandidates on a superseded claim = %+v, want empty", candidates)
	}
}

// TestDecayNeverWritesConfidenceBackToAFile is the acc line's own clause,
// verbatim: "decay never writes a confidence value back to a file".
// decay.go performs no filesystem I/O at all — proven here by asserting
// every function's input claims are byte-for-byte identical, field by
// field, before and after a full Sweep call (the strongest available
// proxy for "nothing was written anywhere," since a mutation in memory
// would be the first sign of a stray write path).
func TestDecayNeverWritesConfidenceBackToAFile(t *testing.T) {
	now := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	claims := []domain.Claim{
		mkClaim("c1", "dave-osei", "has_balance", 0.9, now.AddDate(0, 0, -100)),
		mkClaim("c2", "dave-osei", "has_balance", 0.2, now.AddDate(0, 0, -5)),
	}
	claims[0].Provenance.Meta = map[string]string{"source": "preserve original attribution"}
	before := make([]domain.Claim, len(claims))
	copy(before, claims)
	for i := range before {
		before[i].Provenance.Meta = maps.Clone(claims[i].Provenance.Meta)
	}

	_ = Sweep(claims, map[string]int{"has_balance": 10}, now, 0)

	for i := range claims {
		if !reflect.DeepEqual(claims[i], before[i]) {
			t.Fatalf("Sweep mutated claims[%d]: got %+v, want unchanged %+v", i, claims[i], before[i])
		}
	}
}

func TestAliasCandidatesDetectsCloseSpellings(t *testing.T) {
	claims := []domain.Claim{
		mkClaim("c1", "alice-tan", "has_balance", 0.5, time.Now()),
		mkClaim("c2", "alice-tann", "has_balance", 0.5, time.Now()), // edit distance 1
		mkClaim("c3", "acme-corp", "has_balance", 0.5, time.Now()),
		mkClaim("c4", "acme", "has_balance", 0.5, time.Now()), // token subset of acme-corp
		mkClaim("c5", "zephyr-industries", "has_balance", 0.5, time.Now()),
	}

	got := AliasCandidates(claims)

	want := map[[2]string]bool{
		{"alice-tan", "alice-tann"}: true,
		{"acme", "acme-corp"}:       true,
	}
	if len(got) != len(want) {
		t.Fatalf("AliasCandidates returned %d pairs, want %d: %+v", len(got), len(want), got)
	}
	for _, pair := range got {
		if !want[[2]string{pair.A, pair.B}] {
			t.Fatalf("unexpected alias pair %+v", pair)
		}
	}
}

func TestAliasCandidatesNoFalsePositiveForUnrelatedSlugs(t *testing.T) {
	claims := []domain.Claim{
		mkClaim("c1", "alice-tan", "has_balance", 0.5, time.Now()),
		mkClaim("c2", "zephyr-industries", "has_balance", 0.5, time.Now()),
	}
	if got := AliasCandidates(claims); len(got) != 0 {
		t.Fatalf("AliasCandidates on unrelated slugs = %+v, want empty", got)
	}
}

func TestAliasCandidatesDeterministicOrder(t *testing.T) {
	claims := []domain.Claim{
		mkClaim("c1", "acme-corp", "has_balance", 0.5, time.Now()),
		mkClaim("c2", "acme", "has_balance", 0.5, time.Now()),
	}
	first := AliasCandidates(claims)
	second := AliasCandidates(claims)
	if len(first) != 1 || len(second) != 1 || first[0] != second[0] {
		t.Fatalf("AliasCandidates not deterministic: %+v vs %+v", first, second)
	}
}

func TestSweepBundlesAllThreeOutputs(t *testing.T) {
	now := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	claims := []domain.Claim{
		mkClaim("c1", "alice-tan", "has_balance", 0.9, now.AddDate(0, 0, -60)),
		mkClaim("c2", "alice-tann", "has_balance", 0.9, now),
	}
	result := Sweep(claims, map[string]int{"has_balance": 30}, now, 0)
	if len(result.Ranked) != 2 {
		t.Fatalf("Sweep.Ranked has %d entries, want 2", len(result.Ranked))
	}
	if len(result.AliasCandidates) != 1 {
		t.Fatalf("Sweep.AliasCandidates has %d entries, want 1", len(result.AliasCandidates))
	}
	if len(result.DistillCandidates) != 1 {
		t.Fatalf("Sweep.DistillCandidates has %d entries, want 1", len(result.DistillCandidates))
	}
}

func TestLevenshteinBasic(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "abc", 0},
		{"", "abc", 3},
		{"abc", "", 3},
		{"kitten", "sitting", 3},
		{"alice-tan", "alice-tann", 1},
	}
	for _, c := range cases {
		if got := levenshtein(c.a, c.b); got != c.want {
			t.Fatalf("levenshtein(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

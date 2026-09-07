package reconcile

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/disposition"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/store"
)

func openTestStore(t *testing.T) *disposition.Store {
	t.Helper()
	eng, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("index.Open: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })
	return disposition.NewStore(eng)
}

var reconcileFixedNow = time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

func claimFixture(id, subject, predicate, object string) domain.Claim {
	return domain.Claim{
		ID:          id,
		SubjectSlug: subject,
		Predicate:   predicate,
		Object:      object,
		ObjectKey:   store.NormalizeKey(object),
		Confidence:  0.9,
		State:       domain.StateActive,
		Family:      predicate,
		Provenance: domain.Provenance{
			SourceSHA256: "sha-" + id,
			Span:         "0-10",
			Model:        "test-model@v1",
			ObservedAt:   reconcileFixedNow,
			Actor:        "machine",
		},
	}
}

// TestContradictingBalanceFixtureProducesExactlyOneConflictItem is T2.2's
// acc-line clause: "contradicting-balance fixture -> exactly one A/B
// item with both provenances." Two active claims, same (subject,
// predicate), differing objects, neither carrying a validity window or
// scope qualifier -- the textbook same-time contradiction RFC §10.2
// names.
func TestContradictingBalanceFixtureProducesExactlyOneConflictItem(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	existing := claimFixture("claim-b", "acme-corp", "has_balance", "$500")
	existing.Provenance.SourceSHA256 = "sha-source-b"
	newClaim := claimFixture("claim-a", "acme-corp", "has_balance", "$700")
	newClaim.Provenance.SourceSHA256 = "sha-source-a"

	eng := NewEngine(s)
	detection, item, err := eng.Process(ctx, newClaim, []domain.Claim{existing}, reconcileFixedNow)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if detection.Verdict != VerdictConflict {
		t.Fatalf("Verdict = %q, want %q", detection.Verdict, VerdictConflict)
	}
	if item == nil {
		t.Fatal("item is nil, want exactly one staged disposition item")
	}
	if item.Kind != disposition.KindReconcile {
		t.Fatalf("item.Kind = %q, want %q", item.Kind, disposition.KindReconcile)
	}

	var payload ReconcilePayload
	if err := json.Unmarshal(item.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.A.Provenance.SourceSHA256 != "sha-source-a" {
		t.Errorf("payload.A provenance = %+v, want source A's provenance", payload.A.Provenance)
	}
	if payload.B.Provenance.SourceSHA256 != "sha-source-b" {
		t.Errorf("payload.B provenance = %+v, want source B's provenance", payload.B.Provenance)
	}

	// Exactly one item: List must show exactly one KindReconcile item for
	// this pair, not one per candidate or one per side.
	all, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	count := 0
	for _, it := range all {
		if it.Kind == disposition.KindReconcile {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("staged %d KindReconcile items, want exactly 1", count)
	}
}

// TestOverlappingWindowFixtureProposesWindowCloseNotRefutation is T2.2's
// acc-line clause: "overlapping-window fixture -> window-close proposal,
// not refutation." Two active claims, same (subject, predicate), each
// carrying a determinable and distinct validity window -- the detector
// must recognize temporal supersession rather than flat contradiction.
func TestOverlappingWindowFixtureProposesWindowCloseNotRefutation(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	existing := claimFixture("claim-b", "alice-tan", "works_at", "initech")
	existing.ValidFrom = "2023-01-01"
	newClaim := claimFixture("claim-a", "alice-tan", "works_at", "acme-corp")
	newClaim.ValidFrom = "2025-06-01"

	eng := NewEngine(s)
	detection, item, err := eng.Process(ctx, newClaim, []domain.Claim{existing}, reconcileFixedNow)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if detection.Verdict != VerdictWindowClose {
		t.Fatalf("Verdict = %q, want %q (not %q -- refutation is the wrong call for a temporally distinct window)", detection.Verdict, VerdictWindowClose, VerdictConflict)
	}
	if item == nil {
		t.Fatal("item is nil, want a staged proposal -- window-close is still routed through DISPOSITION, never auto-applied at trust 0")
	}

	var payload ReconcilePayload
	if err := json.Unmarshal(item.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Verdict != VerdictWindowClose {
		t.Fatalf("payload.Verdict = %q, want %q", payload.Verdict, VerdictWindowClose)
	}
}

// TestParseValidFromAcceptsRFC3339 is T2.23's acc-line clause directly:
// "an input with a RFC3339 valid_from (e.g. 2026-09-07T00:00:00Z) parses
// successfully." Before this task, parseValidFrom only accepted
// validDateLayout ("2006-01-02"); an RFC3339 string with a time-of-day
// and "Z" suffix failed to parse at all.
func TestParseValidFromAcceptsRFC3339(t *testing.T) {
	got, ok := parseValidFrom("2026-09-07T00:00:00Z")
	if !ok {
		t.Fatal("parseValidFrom(RFC3339 input) ok = false, want true")
	}
	want := time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("parseValidFrom(RFC3339 input) = %v, want %v", got, want)
	}
}

// TestParseValidFromStillRejectsGarbage guards against a fix that widens
// parseValidFrom into accepting anything -- only the two named layouts
// (validDateLayout and RFC3339) are valid; an unrelated string must still
// fail closed exactly as before.
func TestParseValidFromStillRejectsGarbage(t *testing.T) {
	if _, ok := parseValidFrom("not-a-date"); ok {
		t.Fatal("parseValidFrom(garbage) ok = true, want false")
	}
	if _, ok := parseValidFrom(""); ok {
		t.Fatal("parseValidFrom(empty) ok = true, want false")
	}
}

// TestRFC014FixtureNowProducesWindowCloseNotConflict reproduces T2.18's
// own deliberately-included R-014 fixture
// (evals/corpora/reconcile/labels/R-014.yaml) at the Detect/Engine level:
// the same job-change scenario as
// TestOverlappingWindowFixtureProposesWindowCloseNotRefutation, but with
// ValidFrom written in RFC3339 (time-of-day + "Z" suffix) instead of
// validDateLayout's calendar-day-only format. Before T2.23, parseValidFrom
// rejected this format outright, so Detect fell through to
// VerdictConflict instead of the temporally-correct VerdictWindowClose --
// exactly the false negative T2.18's reconcile eval reported honestly as
// report.VerdictConfusion["window_close"].FN == 1.
func TestRFC014FixtureNowProducesWindowCloseNotConflict(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	existing := claimFixture("claim-b", "alice-tan", "works_at", "initech")
	existing.ValidFrom = "2023-01-01T00:00:00Z"
	newClaim := claimFixture("claim-a", "alice-tan", "works_at", "acme-corp")
	newClaim.ValidFrom = "2025-06-01T00:00:00Z"

	eng := NewEngine(s)
	detection, item, err := eng.Process(ctx, newClaim, []domain.Claim{existing}, reconcileFixedNow)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if detection.Verdict != VerdictWindowClose {
		t.Fatalf("Verdict = %q, want %q (R-014's RFC3339 valid_from must parse and be recognized as a temporally distinct window, not fall through to %q)", detection.Verdict, VerdictWindowClose, VerdictConflict)
	}
	if item == nil {
		t.Fatal("item is nil, want a staged proposal")
	}
}

// TestProjectScopedFixtureReturnsScopedVerdict is T2.2's acc-line clause:
// "project-scoped fixture -> scoped verdict." Two active claims, same
// (subject, predicate), each carrying a distinct explicit scope
// qualifier (RFC's own "true for project X" example) -- both true, no
// DISPOSITION item.
func TestProjectScopedFixtureReturnsScopedVerdict(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	existing := claimFixture("claim-b", "bob-lee", "committed_to", "security review for project phoenix")
	newClaim := claimFixture("claim-a", "bob-lee", "committed_to", "security review for project atlas")

	eng := NewEngine(s)
	detection, item, err := eng.Process(ctx, newClaim, []domain.Claim{existing}, reconcileFixedNow)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if detection.Verdict != VerdictScoped {
		t.Fatalf("Verdict = %q, want %q", detection.Verdict, VerdictScoped)
	}
	if item != nil {
		t.Fatalf("item = %+v, want nil -- a scoped verdict means both claims stay, nothing for a human to review", item)
	}
}

// TestNoClaimStateChangesWithoutADispose is T2.2's acc-line clause: "no
// claim state changes without a dispose (asserted by dump diff)."
// Serializes every input claim before and after Process (across all
// three verdict shapes above, plus a plain agree/corroboration case) and
// asserts byte-identical dumps -- Detect and Candidates are pure reads,
// and Process's only write is Store.Create against the disposition
// queue's own runtime tables, never canonical claim state.
func TestNoClaimStateChangesWithoutADispose(t *testing.T) {
	cases := []struct {
		name     string
		newClaim domain.Claim
		active   []domain.Claim
	}{
		{
			name:     "conflict",
			newClaim: claimFixture("claim-a", "acme-corp", "has_balance", "$700"),
			active:   []domain.Claim{claimFixture("claim-b", "acme-corp", "has_balance", "$500")},
		},
		{
			name: "window_close",
			newClaim: func() domain.Claim {
				c := claimFixture("claim-a", "alice-tan", "works_at", "acme-corp")
				c.ValidFrom = "2025-06-01"
				return c
			}(),
			active: []domain.Claim{func() domain.Claim {
				c := claimFixture("claim-b", "alice-tan", "works_at", "initech")
				c.ValidFrom = "2023-01-01"
				return c
			}()},
		},
		{
			name:     "scoped",
			newClaim: claimFixture("claim-a", "bob-lee", "committed_to", "security review for project atlas"),
			active:   []domain.Claim{claimFixture("claim-b", "bob-lee", "committed_to", "security review for project phoenix")},
		},
		{
			name:     "agree",
			newClaim: claimFixture("claim-a", "carol-diaz", "works_at", "globex"),
			active:   []domain.Claim{claimFixture("claim-b", "carol-diaz", "works_at", "globex")},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := openTestStore(t)
			ctx := context.Background()

			beforeNew, err := json.Marshal(tc.newClaim)
			if err != nil {
				t.Fatalf("marshal newClaim: %v", err)
			}
			beforeActive, err := json.Marshal(tc.active)
			if err != nil {
				t.Fatalf("marshal active: %v", err)
			}

			eng := NewEngine(s)
			if _, _, err := eng.Process(ctx, tc.newClaim, tc.active, reconcileFixedNow); err != nil {
				t.Fatalf("Process: %v", err)
			}

			afterNew, err := json.Marshal(tc.newClaim)
			if err != nil {
				t.Fatalf("marshal newClaim after: %v", err)
			}
			afterActive, err := json.Marshal(tc.active)
			if err != nil {
				t.Fatalf("marshal active after: %v", err)
			}

			if string(beforeNew) != string(afterNew) {
				t.Errorf("newClaim mutated by Process:\nbefore: %s\nafter:  %s", beforeNew, afterNew)
			}
			if string(beforeActive) != string(afterActive) {
				t.Errorf("active claims mutated by Process:\nbefore: %s\nafter:  %s", beforeActive, afterActive)
			}
		})
	}
}

func TestDetectReturnsNeutralAdditiveWhenNoCandidatesShareSubjectPredicate(t *testing.T) {
	newClaim := claimFixture("claim-a", "dana-osei", "prefers", "tea")
	unrelated := claimFixture("claim-b", "dana-osei", "has_role", "engineer")

	d := Detect(newClaim, Candidates(newClaim, []domain.Claim{unrelated}))
	if d.Verdict != VerdictNeutralAdditive {
		t.Fatalf("Verdict = %q, want %q", d.Verdict, VerdictNeutralAdditive)
	}
}

func TestCandidatesExcludesSuperseded(t *testing.T) {
	newClaim := claimFixture("claim-a", "erin-fox", "works_at", "acme-corp")
	superseded := claimFixture("claim-b", "erin-fox", "works_at", "initech")
	superseded.State = domain.StateSuperseded

	cands := Candidates(newClaim, []domain.Claim{superseded})
	if len(cands) != 0 {
		t.Fatalf("Candidates = %v, want none -- a superseded claim is not an active neighbor", cands)
	}
}

func TestCandidatesExcludesSelf(t *testing.T) {
	newClaim := claimFixture("claim-a", "frank-nu", "works_at", "acme-corp")
	cands := Candidates(newClaim, []domain.Claim{newClaim})
	if len(cands) != 0 {
		t.Fatalf("Candidates = %v, want none -- a claim is never its own neighbor", cands)
	}
}

// TestDetectPrioritizesConflictOverAgreeingNeighbor exercises the
// candidate-priority resolution Detect's own doc names: a genuinely
// conflicting neighbor must never be masked by a different neighbor that
// happens to agree.
func TestDetectPrioritizesConflictOverAgreeingNeighbor(t *testing.T) {
	newClaim := claimFixture("claim-a", "grace-lin", "has_balance", "$700")
	agreeing := claimFixture("claim-b", "grace-lin", "has_balance", "$700")
	conflicting := claimFixture("claim-c", "grace-lin", "has_balance", "$500")

	d := Detect(newClaim, Candidates(newClaim, []domain.Claim{agreeing, conflicting}))
	if d.Verdict != VerdictConflict {
		t.Fatalf("Verdict = %q, want %q -- a real conflict must win over an agreeing neighbor", d.Verdict, VerdictConflict)
	}
	if d.Candidate.ID != "claim-c" {
		t.Fatalf("Candidate.ID = %q, want the conflicting claim claim-c", d.Candidate.ID)
	}
}

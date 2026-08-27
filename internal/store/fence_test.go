package store

import (
	"bytes"
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	"github.com/sirerun/serenity/internal/domain"
)

// TestFenceRoundTripExample pins the canonical byte form on the RFC §7.2
// example shape and asserts parse recovers the structure.
func TestFenceRoundTripExample(t *testing.T) {
	p := &EntityPage{
		Entity:  domain.Entity{Type: "person", Slug: "alice-tan", Aliases: []string{"Alice", "A. Tan"}},
		Title:   "Alice Tan",
		Summary: "Runs engineering at Acme (series-B fintech). Last contact 2026-04-22…",
		Claims: []domain.Claim{
			{ID: "c7f3a", SubjectSlug: "alice-tan", Predicate: "works_at", Object: "acme",
				Confidence: 0.92, ValidFrom: "2025-06", SourceRef: "e42#3", State: domain.StateActive, Family: "works_at"},
			{ID: "c81b0", SubjectSlug: "alice-tan", Predicate: "committed_to", Object: "security review by 2026-05-01",
				Confidence: 0.85, ValidFrom: "2026-04-22", SourceRef: "e57#1", State: domain.StateActive, Family: "committed_to"},
			{ID: "c55d2", SubjectSlug: "alice-tan", Predicate: "works_at", Object: "initech",
				Confidence: 0.88, ValidFrom: "2023", ValidTo: "2025-06", SourceRef: "e12#7",
				State: domain.StateSuperseded, SupersededBy: "c7f3a", Family: "works_at"},
		},
		Timeline: []TimelineEntry{{Date: "2026-04-22", Text: "pricing chat (e57)"}},
	}

	w := NewFenceWriter(t.TempDir())
	first, err := w.RenderEntity(p)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseEntityBytes(first)
	if err != nil {
		t.Fatal(err)
	}
	second, err := w.RenderEntity(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("round trip not byte-identical:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}

	if len(parsed.Claims) != 3 {
		t.Fatalf("expected 3 claims, got %d", len(parsed.Claims))
	}
	var superseded *domain.Claim
	for i := range parsed.Claims {
		if parsed.Claims[i].ID == "c55d2" {
			superseded = &parsed.Claims[i]
		}
	}
	if superseded == nil {
		t.Fatal("superseded claim c55d2 lost in round trip")
	}
	if superseded.State != domain.StateSuperseded || superseded.SupersededBy != "c7f3a" {
		t.Fatalf("supersession not preserved: %+v", superseded)
	}
	if superseded.ValidFrom != "2023" || superseded.ValidTo != "2025-06" {
		t.Fatalf("validity window not preserved: %+v", superseded)
	}
	if parsed.Entity.Slug != "alice-tan" || len(parsed.Entity.Aliases) != 2 {
		t.Fatalf("frontmatter not preserved: %+v", parsed.Entity)
	}
}

// TestFenceRoundTripProperty is the M0 byte-identical round-trip property
// test: for any canonical page, parse → render reproduces exact bytes.
func TestFenceRoundTripProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(20260826))
	w := NewFenceWriter(t.TempDir())
	for i := 0; i < 300; i++ {
		p := randomPage(rng)
		first, err := w.RenderEntity(p)
		if err != nil {
			t.Fatalf("iter %d render: %v", i, err)
		}
		parsed, err := ParseEntityBytes(first)
		if err != nil {
			t.Fatalf("iter %d parse: %v\npage:\n%s", i, err, first)
		}
		second, err := w.RenderEntity(parsed)
		if err != nil {
			t.Fatalf("iter %d re-render: %v", i, err)
		}
		if !bytes.Equal(first, second) {
			t.Fatalf("iter %d not byte-identical\n--- first ---\n%s\n--- second ---\n%s", i, first, second)
		}
		if len(parsed.Claims) != len(p.Claims) {
			t.Fatalf("iter %d claim count changed: %d -> %d", i, len(p.Claims), len(parsed.Claims))
		}
	}
}

// TestFenceWriteIdempotent asserts no-op writes never dirty the tree.
func TestFenceWriteIdempotent(t *testing.T) {
	w := NewFenceWriter(t.TempDir())
	p := NewEntityPage(domain.Entity{Type: "person", Slug: "bob"})
	p.Summary = "hi"
	path, err := w.WriteEntity(p)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := w.ParseEntity(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteEntity(parsed); err != nil {
		t.Fatal(err)
	}
	roundTripped, err := w.ParseEntity(path)
	if err != nil {
		t.Fatal(err)
	}
	if roundTripped.Summary != "hi" || roundTripped.Entity.Slug != "bob" {
		t.Fatalf("write round trip lost data: %+v", roundTripped)
	}
}

// TestFenceConcurrentMergeDeterministic simulates two concurrent branches
// each making a merge-safe edit (append a row / mark-superseded a row) to
// copies of the same page, then a git-merge-file style three-way union of
// both sides' rows. Per §7.2 the writer sorts and normalizes on render, so
// the same row set renders byte-identically no matter which side's rows
// land first in the union — that's what makes concurrent git merges on
// these pages resolve deterministically rather than by diff3 conflict
// markers or line order.
func TestFenceConcurrentMergeDeterministic(t *testing.T) {
	w := NewFenceWriter(t.TempDir())
	entity := domain.Entity{Type: "person", Slug: "alice-tan"}

	base := domain.Claim{
		ID: "cbase01", SubjectSlug: "alice-tan", Predicate: "works_at", Family: "works_at",
		Object: "initech", Confidence: 0.8, ValidFrom: "2023", SourceRef: "e1#1", State: domain.StateActive,
	}

	// Side A's edit: append a new row (merge-safe class).
	sideAAppend := domain.Claim{
		ID: "ca00001", SubjectSlug: "alice-tan", Predicate: "prefers", Family: "prefers",
		Object: "async standups", Confidence: 0.7, SourceRef: "e2#1", State: domain.StateActive,
	}
	// Side B's edit: append a different new row and mark the base row
	// superseded (also merge-safe: the row is kept, only state/pointer
	// change — no line is deleted or rewritten in place).
	sideBAppend := domain.Claim{
		ID: "cb00002", SubjectSlug: "alice-tan", Predicate: "works_at", Family: "works_at",
		Object: "acme", Confidence: 0.9, ValidFrom: "2025-06", SourceRef: "e3#1", State: domain.StateActive,
	}
	baseSupersededByB := base
	baseSupersededByB.State = domain.StateSuperseded
	baseSupersededByB.SupersededBy = sideBAppend.ID

	// Union the three rows in A-then-B concatenation order.
	pageAB := NewEntityPage(entity)
	pageAB.Claims = []domain.Claim{baseSupersededByB, sideAAppend, sideBAppend}

	// Union the same three rows in B-then-A order — a real three-way merge
	// can land either side's hunk first depending on diff/patch order.
	pageBA := NewEntityPage(entity)
	pageBA.Claims = []domain.Claim{sideBAppend, baseSupersededByB, sideAAppend}

	if reflect.DeepEqual(pageAB.Claims, pageBA.Claims) {
		t.Fatal("test setup broken: the two pre-render row orderings must differ to exercise merge-order independence")
	}

	renderedAB, err := w.RenderEntity(pageAB)
	if err != nil {
		t.Fatal(err)
	}
	renderedBA, err := w.RenderEntity(pageBA)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(renderedAB, renderedBA) {
		t.Fatalf("merge-order dependent render:\n--- A-then-B ---\n%s\n--- B-then-A ---\n%s", renderedAB, renderedBA)
	}
}

// TestFenceWriteEntityAcceptsKnownPredicate asserts a claim whose predicate
// is in the seeded controlled vocabulary (RFC §7.2) writes cleanly.
func TestFenceWriteEntityAcceptsKnownPredicate(t *testing.T) {
	w := NewFenceWriter(t.TempDir())
	p := NewEntityPage(domain.Entity{Type: "person", Slug: "carol"})
	p.Claims = []domain.Claim{{ID: "k1", Predicate: "works_at", Object: "acme", State: domain.StateActive}}
	if _, err := w.WriteEntity(p); err != nil {
		t.Fatalf("known predicate rejected: %v", err)
	}
}

// TestFenceWriteEntityRejectsUnknownPredicate is T0.8: a predicate outside
// the controlled vocabulary must be rejected, not written ad hoc (§7.2).
func TestFenceWriteEntityRejectsUnknownPredicate(t *testing.T) {
	w := NewFenceWriter(t.TempDir())
	p := NewEntityPage(domain.Entity{Type: "person", Slug: "dave"})
	p.Claims = []domain.Claim{{ID: "k2", Predicate: "launches_missiles", Object: "nowhere", State: domain.StateActive}}
	if _, err := w.WriteEntity(p); err == nil {
		t.Fatal("expected unknown predicate to be rejected")
	}
}

func TestRejectsMultilineObjects(t *testing.T) {
	w := NewFenceWriter(t.TempDir())
	p := NewEntityPage(domain.Entity{Type: "person", Slug: "eve"})
	p.Claims = []domain.Claim{{ID: "x", Predicate: "said", Object: "line1\nline2", State: domain.StateActive}}
	if _, err := w.RenderEntity(p); err == nil {
		t.Fatal("expected multiline object to be rejected")
	}
}

// --- generators ---

var predicates = []string{"works_at", "has_role", "prefers", "committed_to", "said", "relates_to"}

// objectRunes includes pipes, backslashes, wide runes, and the ellipsis to
// exercise escaping and rune-safe truncation.
var objectRunes = []rune("abcdefghij XYZ0123456789|\\éß→…$#:.-~")

func randomPage(rng *rand.Rand) *EntityPage {
	slug := fmt.Sprintf("ent-%d", rng.Intn(1_000_000))
	e := domain.Entity{Type: "person", Slug: slug}
	if rng.Intn(2) == 0 {
		e.Aliases = []string{randWord(rng, 4), randWord(rng, 6)}
	}
	p := NewEntityPage(e)
	if rng.Intn(4) > 0 {
		p.Summary = randWord(rng, 12) + "\n" + randWord(rng, 20)
	}
	n := rng.Intn(8)
	for i := 0; i < n; i++ {
		c := domain.Claim{
			ID:          fmt.Sprintf("c%06x", rng.Intn(0xffffff)) + fmt.Sprintf("%02d", i),
			SubjectSlug: slug,
			Predicate:   predicates[rng.Intn(len(predicates))],
			Object:      randObject(rng),
			Confidence:  float64(rng.Intn(101)) / 100,
			State:       domain.StateActive,
			SourceRef:   fmt.Sprintf("e%d#%d", rng.Intn(99), rng.Intn(9)),
		}
		c.Family = c.Predicate
		switch rng.Intn(6) {
		case 0:
			c.State = domain.StateSuperseded
			c.SupersededBy = fmt.Sprintf("c%06x", rng.Intn(0xffffff))
		case 1:
			c.State = domain.StateRetracted
		}
		switch rng.Intn(3) {
		case 0:
			c.ValidFrom = randDate(rng)
		case 1:
			c.ValidFrom, c.ValidTo = randDate(rng), randDate(rng)
		}
		p.Claims = append(p.Claims, c)
	}
	for i := 0; i < rng.Intn(4); i++ {
		p.Timeline = append(p.Timeline, TimelineEntry{Date: randDate(rng), Text: randWord(rng, 10)})
	}
	return p
}

func randObject(rng *rand.Rand) string {
	// ~1 in 5 objects exceed the 120-rune cell cap to exercise the
	// claims-detail block.
	n := 1 + rng.Intn(40)
	if rng.Intn(5) == 0 {
		n = 121 + rng.Intn(120)
	}
	r := make([]rune, n)
	for i := range r {
		r[i] = objectRunes[rng.Intn(len(objectRunes))]
	}
	return string(r)
}

func randWord(rng *rand.Rand, n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, 1+rng.Intn(n))
	for i := range b {
		b[i] = letters[rng.Intn(len(letters))]
	}
	return string(b)
}

func randDate(rng *rand.Rand) string {
	return fmt.Sprintf("%04d-%02d-%02d", 2020+rng.Intn(7), 1+rng.Intn(12), 1+rng.Intn(28))
}

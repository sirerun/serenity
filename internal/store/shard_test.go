package store

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/domain"
)

// TestShard10KProperty is the M0 acceptance property test (§7.2a):
// 10,000 claims into one shard family — append, resolve, rebuild, merge
// two divergent copies — with bounded file size, deterministic merges,
// and identical rebuilt heads.
func TestShard10KProperty(t *testing.T) {
	const total = 10_000
	rng := rand.New(rand.NewSource(20260826))
	root := t.TempDir()
	s := NewShardStore(root)
	const slug, family = "checking-acct", "has_balance"
	// 10,000 claims crammed into ONE shard file is a bulk/shard-scale
	// stress case, not the "per-entity population" DefaultIDWidth (32
	// bits) is sized for (ADR 004 D2) -- at that volume the birthday bound
	// gives a non-negligible chance of a genuine collision (this test hit
	// one at DefaultIDWidth with its fixed seed, correctly caught by
	// ErrIDCollision). Use a wider id here; DefaultIDWidth stays the
	// standard elsewhere.
	const shardTestIDWidth = 16

	// expected[key] = id of the live head, maintained independently of the
	// resolver so the test is not tautological.
	expected := map[string]string{}
	liveKeys := []string{}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	var all []domain.Claim
	for i := 0; i < total; i++ {
		obs := base.Add(time.Duration(i) * time.Minute)
		var c domain.Claim
		switch {
		case len(liveKeys) > 0 && rng.Intn(100) < 20: // supersession
			key := liveKeys[rng.Intn(len(liveKeys))]
			c = domain.Claim{
				SubjectSlug: slug, Predicate: family, Family: family,
				Object:     fmt.Sprintf("%d.%02d usd (%s)", rng.Intn(10000), rng.Intn(100), key),
				ObjectKey:  key,
				Confidence: 0.9,
				State:      domain.StateActive,
				Supersedes: expected[key],
				Provenance: domain.Provenance{ObservedAt: obs, Actor: "machine", SourceSHA256: fmt.Sprintf("src-%d", i)},
			}
			c.ID = DerivedID(slug, family, key, "", c.Provenance.SourceSHA256, shardTestIDWidth)
			expected[key] = c.ID
		case len(liveKeys) > 20 && rng.Intn(100) < 2: // retraction tombstone
			ki := rng.Intn(len(liveKeys))
			key := liveKeys[ki]
			c = domain.Claim{
				SubjectSlug: slug, Predicate: family, Family: family,
				ID: expected[key], ObjectKey: key,
				State:      domain.StateRetracted,
				Provenance: domain.Provenance{ObservedAt: obs, Actor: "human:qa"},
			}
			delete(expected, key)
			liveKeys = append(liveKeys[:ki], liveKeys[ki+1:]...)
		default: // fresh key
			key := fmt.Sprintf("account-%06d", i)
			c = domain.Claim{
				SubjectSlug: slug, Predicate: family, Family: family,
				Object:     fmt.Sprintf("%d.%02d usd (%s)", rng.Intn(10000), rng.Intn(100), key),
				ObjectKey:  key,
				Confidence: 0.9,
				State:      domain.StateActive,
				Provenance: domain.Provenance{ObservedAt: obs, Actor: "machine", SourceSHA256: fmt.Sprintf("src-%d", i)},
			}
			c.ID = DerivedID(slug, family, key, "", c.Provenance.SourceSHA256, shardTestIDWidth)
			expected[key] = c.ID
			liveKeys = append(liveKeys, key)
		}
		if err := s.Append(c); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
		all = append(all, c)
	}

	// Bounded file size.
	fi, err := os.Stat(s.PathFor(slug, family))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() > 16*1024*1024 {
		t.Fatalf("shard file unbounded: %d bytes for %d claims", fi.Size(), total)
	}
	t.Logf("shard size for %d claims: %d bytes (%d live heads)", total, fi.Size(), len(expected))

	// Resolve matches the independently-tracked expectation.
	heads, err := s.ResolveHeads(slug, family)
	if err != nil {
		t.Fatal(err)
	}
	assertHeads(t, "resolve", heads, expected)

	// Rebuild: a fresh store over the same bytes resolves identically.
	heads2, err := NewShardStore(root).ResolveHeads(slug, family)
	if err != nil {
		t.Fatal(err)
	}
	assertHeads(t, "rebuild", heads2, expected)

	// Order independence: resolution over reversed lines is identical —
	// this is what makes git line-union merges deterministic (§7.7).
	rev := make([]domain.Claim, len(all))
	for i, c := range all {
		rev[len(all)-1-i] = c
	}
	assertHeads(t, "reversed", ResolveHeadLines(rev), expected)

	// Merge two divergent copies (overlapping halves) — deterministic,
	// identical heads.
	a, b := all[:7000], all[3000:]
	merged := MergeLines(a, b)
	if len(merged) != len(all) {
		t.Fatalf("merge dedup wrong: %d lines, want %d", len(merged), len(all))
	}
	assertHeads(t, "merged", ResolveHeadLines(merged), expected)

	// Compact, then resolve/rebuild over the compacted on-disk state
	// (§7.7): the dead lines move to the archive shard, but a FRESH store
	// resolving the live shard alone must still reach the identical heads.
	if _, err := s.Compact(slug, family); err != nil {
		t.Fatal(err)
	}
	heads3, err := NewShardStore(root).ResolveHeads(slug, family)
	if err != nil {
		t.Fatal(err)
	}
	assertHeads(t, "compacted-rebuild", heads3, expected)
}

func assertHeads(t *testing.T, label string, got map[string]domain.Claim, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d heads, want %d", label, len(got), len(want))
	}
	for key, id := range want {
		h, ok := got[key]
		if !ok {
			t.Fatalf("%s: missing head for key %q", label, key)
		}
		if h.ID != id {
			t.Fatalf("%s: head for %q = %s, want %s", label, key, h.ID, id)
		}
	}
}

// TestShardCompact: superseded/retracted lines move to the archive shard;
// live heads are untouched; the operation is explicit (§7.7).
func TestShardCompact(t *testing.T) {
	root := t.TempDir()
	s := NewShardStore(root)
	const slug, family = "acct", "has_balance"
	obs := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	c1 := domain.Claim{SubjectSlug: slug, Predicate: family, Family: family,
		Object: "100.00 usd", ObjectKey: "k1", State: domain.StateActive, ID: "aaaa0001",
		Provenance: domain.Provenance{ObservedAt: obs}}
	c2 := domain.Claim{SubjectSlug: slug, Predicate: family, Family: family,
		Object: "200.00 usd", ObjectKey: "k1", State: domain.StateActive, ID: "aaaa0002", Supersedes: "aaaa0001",
		Provenance: domain.Provenance{ObservedAt: obs.Add(time.Hour)}}
	for _, c := range []domain.Claim{c1, c2} {
		if err := s.Append(c); err != nil {
			t.Fatal(err)
		}
	}

	moved, err := s.Compact(slug, family)
	if err != nil {
		t.Fatal(err)
	}
	if moved != 1 {
		t.Fatalf("expected 1 archived line, got %d", moved)
	}
	lines, err := s.Lines(slug, family)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0].ID != "aaaa0002" {
		t.Fatalf("live shard wrong after compact: %+v", lines)
	}
	arch, err := readShardFile(s.PathFor(slug, family+".archive"))
	if err != nil {
		t.Fatal(err)
	}
	if len(arch) != 1 || arch[0].ID != "aaaa0001" {
		t.Fatalf("archive wrong after compact: %+v", arch)
	}
	heads, err := s.ResolveHeads(slug, family)
	if err != nil {
		t.Fatal(err)
	}
	if len(heads) != 1 || heads["k1"].ID != "aaaa0002" {
		t.Fatalf("heads changed by compact: %+v", heads)
	}
}

// TestShardAppendAcceptsKnownPredicate asserts a claim whose family is in
// the seeded controlled vocabulary (RFC §7.2) appends cleanly.
func TestShardAppendAcceptsKnownPredicate(t *testing.T) {
	s := NewShardStore(t.TempDir())
	c := domain.Claim{SubjectSlug: "x", Predicate: "has_balance", Family: "has_balance",
		Object: "10.00 usd", State: domain.StateActive}
	if err := s.Append(c); err != nil {
		t.Fatalf("known predicate rejected: %v", err)
	}
}

// TestShardAppendRejectsUnknownPredicate is T0.8: a family outside the
// controlled vocabulary must be rejected, not appended ad hoc (§7.2).
func TestShardAppendRejectsUnknownPredicate(t *testing.T) {
	s := NewShardStore(t.TempDir())
	c := domain.Claim{SubjectSlug: "x", Predicate: "launches_missiles", Family: "launches_missiles",
		Object: "10.00 usd", State: domain.StateActive}
	if err := s.Append(c); !errors.Is(err, ErrUnknownPredicate) {
		t.Fatalf("Append with unknown predicate: got err=%v, want errors.Is(err, ErrUnknownPredicate)", err)
	}
	if lines, err := s.Lines(c.SubjectSlug, c.Family); err != nil || len(lines) != 0 {
		t.Fatalf("Append must not write a shard line when the predicate is rejected: lines=%v err=%v", lines, err)
	}
}

func TestShardAppendDerivesID(t *testing.T) {
	s := NewShardStore(t.TempDir())
	c := domain.Claim{SubjectSlug: "x", Predicate: "costs", Family: "costs",
		Object: "12.50", State: domain.StateActive,
		Provenance: domain.Provenance{SourceSHA256: "s1"}}
	if err := s.Append(c); err != nil {
		t.Fatal(err)
	}
	lines, err := s.Lines("x", "costs")
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0].ID == "" || lines[0].ObjectKey != "12.5" {
		t.Fatalf("id/object-key not derived: %+v", lines)
	}
}

// TestShardAppendIDCollision: two claims with different identity tuples
// (ADR 004 D2: subject, predicate, object key, valid_from, source ref)
// forced onto the same id — via a deliberately narrow DerivedID width, so a
// real collision is cheap and deterministic to produce — get
// ErrIDCollision from Append, never a silent overwrite (§7.2).
func TestShardAppendIDCollision(t *testing.T) {
	const slug, family = "acme-corp", "works_at"
	const width = 1 // 16 possible ids: a collision is easy to force

	// Find two distinct source refs whose DerivedID(width=1) collides.
	// Deterministic: sha256 has no randomness, so this is reproducible
	// across every run.
	var refA, refB string
	seen := map[string]string{}
	for i := 0; ; i++ {
		ref := fmt.Sprintf("src-%d", i)
		id := DerivedID(slug, family, "acme", "2026-01", ref, width)
		if prior, ok := seen[id]; ok {
			refA, refB = prior, ref
			break
		}
		seen[id] = ref
		if i > 10_000 {
			t.Fatal("no id collision found in 10000 tries at width 1 -- DerivedID changed?")
		}
	}

	s := NewShardStore(t.TempDir())
	first := domain.Claim{
		SubjectSlug: slug, Predicate: family, Family: family,
		Object: "first", ObjectKey: "acme", ValidFrom: "2026-01",
		Confidence: 0.9, State: domain.StateActive,
		ID:         DerivedID(slug, family, "acme", "2026-01", refA, width),
		Provenance: domain.Provenance{SourceSHA256: refA},
	}
	if err := s.Append(first); err != nil {
		t.Fatalf("first append: %v", err)
	}

	second := domain.Claim{
		SubjectSlug: slug, Predicate: family, Family: family,
		Object: "second", ObjectKey: "acme", ValidFrom: "2026-01",
		Confidence: 0.9, State: domain.StateActive,
		ID:         DerivedID(slug, family, "acme", "2026-01", refB, width),
		Provenance: domain.Provenance{SourceSHA256: refB},
	}
	if second.ID != first.ID {
		t.Fatalf("test setup: ids should collide, got %s and %s", first.ID, second.ID)
	}
	err := s.Append(second)
	if !errors.Is(err, ErrIDCollision) {
		t.Fatalf("Append with colliding id/differing tuple = %v, want ErrIDCollision", err)
	}

	lines, err := s.Lines(slug, family)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0].Object != "first" {
		t.Fatalf("collision must not overwrite: %+v", lines)
	}

	// A repeated observation of the SAME tuple (identical source ref too)
	// is not a collision -- Append must accept it.
	if err := s.Append(first); err != nil {
		t.Fatalf("re-appending the identical tuple must not error: %v", err)
	}
}

// TestShardRolloverOpensNumberedSegmentAndResolveHeadsSpansBoth is T2.9's
// own acc-line clause: "a shard crossing rollover size opens
// <family>.N.jsonl and ResolveHeads spans both."
func TestShardRolloverOpensNumberedSegmentAndResolveHeadsSpansBoth(t *testing.T) {
	root := t.TempDir()
	s := NewShardStore(root)
	s.RolloverBytes = 10 // any single real line already exceeds this
	const slug, family = "acct-9", "has_balance"
	obs := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	// Append takes a claim by value and derives its id internally without
	// mutating the caller's copy (TestShardAppendDerivesID's own
	// convention), so ids are set explicitly here rather than read back
	// off c1/c2 after Append returns.
	c1 := domain.Claim{SubjectSlug: slug, Predicate: family, Family: family,
		Object: "100.00 usd", ObjectKey: "k1", Confidence: 0.9, State: domain.StateActive, ID: "id-c1",
		Provenance: domain.Provenance{ObservedAt: obs, Actor: "machine", SourceSHA256: "src-1"}}
	if err := s.Append(c1); err != nil {
		t.Fatalf("append c1: %v", err)
	}
	// c1 lands in the base file, which the tiny threshold now counts as
	// already full -- the next append must open a new numbered segment
	// rather than keep growing the base file.
	c2 := domain.Claim{SubjectSlug: slug, Predicate: family, Family: family,
		Object: "50.00 usd", ObjectKey: "k2", Confidence: 0.9, State: domain.StateActive, ID: "id-c2",
		Provenance: domain.Provenance{ObservedAt: obs.Add(time.Minute), Actor: "machine", SourceSHA256: "src-2"}}
	if err := s.Append(c2); err != nil {
		t.Fatalf("append c2: %v", err)
	}

	basePath := s.PathFor(slug, family)
	seg1Path := filepath.Join(filepath.Dir(basePath), family+".1.jsonl")
	if _, err := os.Stat(basePath); err != nil {
		t.Fatalf("base segment missing: %v", err)
	}
	if _, err := os.Stat(seg1Path); err != nil {
		t.Fatalf("expected rollover to open %s: %v", seg1Path, err)
	}

	baseLines, err := readShardFile(basePath)
	if err != nil || len(baseLines) != 1 || baseLines[0].ID != c1.ID {
		t.Fatalf("base segment content wrong: %+v err=%v", baseLines, err)
	}
	seg1Lines, err := readShardFile(seg1Path)
	if err != nil || len(seg1Lines) != 1 || seg1Lines[0].ID != c2.ID {
		t.Fatalf("segment 1 content wrong: %+v err=%v", seg1Lines, err)
	}

	// ResolveHeads spans both segments -- neither claim shadows the
	// other since they resolve to distinct object keys.
	heads, err := s.ResolveHeads(slug, family)
	if err != nil {
		t.Fatal(err)
	}
	if len(heads) != 2 || heads["k1"].ID != c1.ID || heads["k2"].ID != c2.ID {
		t.Fatalf("ResolveHeads did not span both segments: %+v", heads)
	}

	// Lines returns both, base segment first (append order).
	lines, err := s.Lines(slug, family)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 2 || lines[0].ID != c1.ID || lines[1].ID != c2.ID {
		t.Fatalf("Lines did not span both segments in order: %+v", lines)
	}
}

// TestShardRolloverOpensMultipleSegmentsInAscendingOrder confirms a third
// rollover opens <family>.2.jsonl (not, say, overwriting .1.jsonl or
// jumping straight to some other number), and that Lines' cross-segment
// order tracks append order across three files, not just two.
func TestShardRolloverOpensMultipleSegmentsInAscendingOrder(t *testing.T) {
	root := t.TempDir()
	s := NewShardStore(root)
	s.RolloverBytes = 10
	const slug, family = "acct-10", "has_balance"
	obs := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	var ids []string
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("id-%d", i)
		c := domain.Claim{SubjectSlug: slug, Predicate: family, Family: family,
			Object:     fmt.Sprintf("%d.00 usd", i),
			ObjectKey:  fmt.Sprintf("k%d", i),
			Confidence: 0.9, State: domain.StateActive, ID: id,
			Provenance: domain.Provenance{ObservedAt: obs.Add(time.Duration(i) * time.Minute), Actor: "machine", SourceSHA256: fmt.Sprintf("src-%d", i)},
		}
		if err := s.Append(c); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
		ids = append(ids, id)
	}

	dir := filepath.Dir(s.PathFor(slug, family))
	for _, name := range []string{family + ".jsonl", family + ".1.jsonl", family + ".2.jsonl"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("expected segment %s: %v", name, err)
		}
	}

	lines, err := s.Lines(slug, family)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 3 {
		t.Fatalf("Lines = %d lines, want 3", len(lines))
	}
	for i, id := range ids {
		if lines[i].ID != id {
			t.Fatalf("line %d = %s, want %s (segment read order must match append order)", i, lines[i].ID, id)
		}
	}
}

// TestShardAppendIDCollisionAcrossRolledOverSegments: the id registry
// (ADR 004 D2) must still catch a collision even when the offending
// prior id lives in an earlier segment than the one the new line would
// land in.
func TestShardAppendIDCollisionAcrossRolledOverSegments(t *testing.T) {
	root := t.TempDir()
	s := NewShardStore(root)
	s.RolloverBytes = 10
	const slug, family = "acct-11", "has_balance"
	const width = 1 // 16 possible ids: a collision is easy to force

	// Find two distinct source refs whose DerivedID(width=1) collides
	// (same brute-force approach TestShardAppendIDCollision uses --
	// deterministic since sha256 has no randomness).
	var refA, refB string
	seen := map[string]string{}
	for i := 0; ; i++ {
		ref := fmt.Sprintf("src-%d", i)
		id := DerivedID(slug, family, "acme", "2026-01", ref, width)
		if prior, ok := seen[id]; ok {
			refA, refB = prior, ref
			break
		}
		seen[id] = ref
		if i > 10_000 {
			t.Fatal("no id collision found in 10000 tries at width 1 -- DerivedID changed?")
		}
	}

	first := domain.Claim{SubjectSlug: slug, Predicate: family, Family: family,
		Object: "first", ObjectKey: "acme", ValidFrom: "2026-01",
		Confidence: 0.9, State: domain.StateActive,
		ID:         DerivedID(slug, family, "acme", "2026-01", refA, width),
		Provenance: domain.Provenance{SourceSHA256: refA},
	}
	if err := s.Append(first); err != nil {
		t.Fatalf("first append: %v", err)
	}
	second := domain.Claim{SubjectSlug: slug, Predicate: family, Family: family,
		Object: "second", ObjectKey: "acme", ValidFrom: "2026-01",
		Confidence: 0.9, State: domain.StateActive,
		ID:         DerivedID(slug, family, "acme", "2026-01", refB, width),
		Provenance: domain.Provenance{SourceSHA256: refB},
	}
	if second.ID != first.ID {
		t.Fatalf("test setup: ids should collide, got %s and %s", first.ID, second.ID)
	}
	// The base segment is already over the tiny threshold, so this
	// append would otherwise open segment 1 -- the collision check must
	// still fire before that ever matters.
	if err := s.Append(second); !errors.Is(err, ErrIDCollision) {
		t.Fatalf("Append with colliding id/differing tuple across segments = %v, want ErrIDCollision", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(s.PathFor(slug, family)), family+".1.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("a refused collision must not open a new segment either: err=%v", err)
	}
}

// TestShardCompactConsolidatesRolledOverSegments: Compact must span every
// segment a family has rolled over into, write kept (live-head) lines
// back to the single base file, and remove the other segments -- proving
// the rebuild invariant (§7.7) survives rollover and compaction together,
// not just compaction alone (TestShard10KProperty already covers compact
// without rollover).
func TestShardCompactConsolidatesRolledOverSegments(t *testing.T) {
	root := t.TempDir()
	s := NewShardStore(root)
	s.RolloverBytes = 10
	const slug, family = "acct-12", "has_balance"
	obs := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	c1 := domain.Claim{SubjectSlug: slug, Predicate: family, Family: family,
		Object: "100.00 usd", ObjectKey: "k1", State: domain.StateActive, ID: "aaaa0001",
		Provenance: domain.Provenance{ObservedAt: obs}}
	if err := s.Append(c1); err != nil {
		t.Fatal(err)
	}
	c2 := domain.Claim{SubjectSlug: slug, Predicate: family, Family: family,
		Object: "200.00 usd", ObjectKey: "k1", State: domain.StateActive, ID: "aaaa0002", Supersedes: "aaaa0001",
		Provenance: domain.Provenance{ObservedAt: obs.Add(time.Hour)}}
	if err := s.Append(c2); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Dir(s.PathFor(slug, family))
	seg1 := filepath.Join(dir, family+".1.jsonl")
	if _, err := os.Stat(seg1); err != nil {
		t.Fatalf("test setup: expected rollover to open segment 1: %v", err)
	}

	moved, err := s.Compact(slug, family)
	if err != nil {
		t.Fatal(err)
	}
	if moved != 1 {
		t.Fatalf("moved = %d, want 1", moved)
	}

	if _, err := os.Stat(seg1); !os.IsNotExist(err) {
		t.Fatalf("expected segment 1 to be removed by Compact, got err=%v", err)
	}

	lines, err := s.Lines(slug, family)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 || lines[0].ID != "aaaa0002" {
		t.Fatalf("post-compact live shard wrong: %+v", lines)
	}

	arch, err := readShardFile(s.PathFor(slug, family+".archive"))
	if err != nil {
		t.Fatal(err)
	}
	if len(arch) != 1 || arch[0].ID != "aaaa0001" {
		t.Fatalf("archive wrong: %+v", arch)
	}

	heads, err := NewShardStore(root).ResolveHeads(slug, family)
	if err != nil {
		t.Fatal(err)
	}
	if len(heads) != 1 || heads["k1"].ID != "aaaa0002" {
		t.Fatalf("rebuilt heads wrong: %+v", heads)
	}
}

// TestShardDefaultRolloverNotTriggeredByOrdinaryUse guards against a
// pathologically small DefaultRolloverBytes: ordinary small-scale use
// (well under TestShard10KProperty's own ~3 MB / 10,000 claims) must
// never open a second segment under the real default.
func TestShardDefaultRolloverNotTriggeredByOrdinaryUse(t *testing.T) {
	root := t.TempDir()
	s := NewShardStore(root) // RolloverBytes left unset -> DefaultRolloverBytes
	const slug, family = "acct-13", "has_balance"
	for i := 0; i < 50; i++ {
		c := domain.Claim{SubjectSlug: slug, Predicate: family, Family: family,
			Object: fmt.Sprintf("%d.00 usd", i), ObjectKey: fmt.Sprintf("k%d", i),
			State:      domain.StateActive,
			Provenance: domain.Provenance{SourceSHA256: fmt.Sprintf("src-%d", i)},
		}
		if err := s.Append(c); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	dir := filepath.Dir(s.PathFor(slug, family))
	if _, err := os.Stat(filepath.Join(dir, family+".1.jsonl")); !os.IsNotExist(err) {
		t.Fatalf("50 small claims should not trigger the default rollover threshold, but segment 1 exists (err=%v)", err)
	}
}

func TestShardFamiliesDeduplicatesNumberedSegments(t *testing.T) {
	s := NewShardStore(t.TempDir())
	s.RolloverBytes = 1
	for i := 0; i < 3; i++ {
		c := domain.Claim{ID: fmt.Sprintf("c%d", i), SubjectSlug: "account", Predicate: "has_balance", Family: "has_balance", Object: fmt.Sprintf("balance%d", i), State: domain.StateActive}
		if err := s.Append(c); err != nil {
			t.Fatal(err)
		}
	}
	families, err := s.Families("account")
	if err != nil {
		t.Fatal(err)
	}
	if len(families) != 1 || families[0] != "has_balance" {
		t.Fatalf("segments reported as families: %v", families)
	}
	if err := os.Remove(s.PathFor("account", "has_balance")); err != nil {
		t.Fatal(err)
	}
	families, err = s.Families("account")
	if err != nil {
		t.Fatal(err)
	}
	if len(families) != 1 || families[0] != "has_balance" {
		t.Fatalf("remaining segments lost their family: %v", families)
	}
}

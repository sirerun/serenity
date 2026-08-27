package store

import (
	"errors"
	"fmt"
	"math/rand"
	"os"
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

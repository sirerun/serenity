package store

import (
	"math/rand"
	"testing"
)

func TestNormalizeKey(t *testing.T) {
	cases := map[string]string{
		"  Acme  Corp ":    "acme corp",
		"ACME":             "acme",
		"2026/8/3":         "2026-08-03",
		"2026-08-03":       "2026-08-03",
		"paid on 2026/1/9": "paid on 2026-01-09",
		"0.50":             "0.5",
		"42":               "42",
		"1234.5600":        "1234.56",
		"a\tb\n c":         "a b c",
		"":                 "",
		"$1,200":           "$1,200", // not a bare number; left alone
	}
	for in, want := range cases {
		if got := NormalizeKey(in); got != want {
			t.Errorf("NormalizeKey(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestNormalizeKeyIdempotent: normalization is a fixed point — claim-id
// derivation depends on it (§7.2).
func TestNormalizeKeyIdempotent(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	runes := []rune("aA \t9/8-20261.é|")
	for i := 0; i < 2000; i++ {
		n := rng.Intn(30)
		r := make([]rune, n)
		for j := range r {
			r[j] = runes[rng.Intn(len(runes))]
		}
		once := NormalizeKey(string(r))
		twice := NormalizeKey(once)
		if once != twice {
			t.Fatalf("not idempotent: %q -> %q -> %q", string(r), once, twice)
		}
	}
}

func TestDerivedIDStable(t *testing.T) {
	a := DerivedID("alice-tan", "works_at", "acme", "2025-06", "e42", DefaultIDWidth)
	b := DerivedID("alice-tan", "works_at", "acme", "2025-06", "e42", DefaultIDWidth)
	if a != b {
		t.Fatalf("same inputs must derive same id: %s != %s", a, b)
	}
	if len(a) != 8 {
		t.Fatalf("expected 8 hex chars, got %q", a)
	}
	// Same logical claim from a different source gets a different id by
	// design — that is corroboration (§7.2).
	c := DerivedID("alice-tan", "works_at", "acme", "2025-06", "e57", DefaultIDWidth)
	if a == c {
		t.Fatal("different source_ref must derive different id")
	}
}

// TestDerivedIDWidth: width controls the returned id's hex length, and an
// out-of-range width falls back to DefaultIDWidth (ADR 004 D2).
func TestDerivedIDWidth(t *testing.T) {
	cases := []struct {
		width int
		want  int
	}{
		{1, 1},
		{16, 16},
		{64, 64},
		{0, DefaultIDWidth},
		{-1, DefaultIDWidth},
		{65, DefaultIDWidth},
	}
	for _, tc := range cases {
		id := DerivedID("alice-tan", "works_at", "acme", "2025-06", "e42", tc.width)
		if len(id) != tc.want {
			t.Errorf("width %d: got id %q (len %d), want len %d", tc.width, id, len(id), tc.want)
		}
	}
	// Widening must not change the leading characters — a migration only
	// ever appends more of the same digest, it never re-derives.
	narrow := DerivedID("alice-tan", "works_at", "acme", "2025-06", "e42", 8)
	wide := DerivedID("alice-tan", "works_at", "acme", "2025-06", "e42", 16)
	if wide[:8] != narrow {
		t.Fatalf("widening must extend the same digest: narrow=%s wide=%s", narrow, wide)
	}
}

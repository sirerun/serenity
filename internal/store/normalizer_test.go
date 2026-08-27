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
	a := DerivedID("alice-tan", "works_at", "acme", "2025-06", "e42")
	b := DerivedID("alice-tan", "works_at", "acme", "2025-06", "e42")
	if a != b {
		t.Fatalf("same inputs must derive same id: %s != %s", a, b)
	}
	if len(a) != 8 {
		t.Fatalf("expected 8 hex chars, got %q", a)
	}
	// Same logical claim from a different source gets a different id by
	// design — that is corroboration (§7.2).
	c := DerivedID("alice-tan", "works_at", "acme", "2025-06", "e57")
	if a == c {
		t.Fatal("different source_ref must derive different id")
	}
}

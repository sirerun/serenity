package domain

import (
	"strings"
	"testing"
)

// TestValidSlug pins the slug grammar to ^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$
// and nothing else (deep review SEC-H03): every shape that could become a
// directory name, a dot-segment or a YAML key is refused, not repaired.
func TestValidSlug(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"single letter", "a", true},
		{"single digit", "7", true},
		{"two chars", "ab", true},
		{"hyphenated", "alice-tan", true},
		{"digits and hyphens", "a1-b2-c3", true},
		{"double hyphen inside", "a--b", true},
		{"length 64", strings.Repeat("a", 64), true},
		{"length 64 with hyphens", "a" + strings.Repeat("-", 62) + "b", true},

		{"empty", "", false},
		{"length 65", strings.Repeat("a", 65), false},
		{"leading hyphen", "-a", false},
		{"trailing hyphen", "a-", false},
		{"only hyphen", "-", false},
		{"uppercase", "Alice", false},
		{"underscore", "alice_tan", false},
		{"space", "alice tan", false},
		{"dot", "alice.tan", false},
		{"dot segment", ".", false},
		{"dot dot segment", "..", false},
		{"dot dot prefix", "../etc", false},
		{"slash", "people/alice", false},
		{"backslash", "people\\alice", false},
		{"colon", "alice:tan", false},
		{"newline", "alice\ntan", false},
		{"newline yaml injection", "acme\ntype: person\naliases: [alice-tan]", false},
		{"carriage return", "alice\rtan", false},
		{"tab", "alice\ttan", false},
		{"nul", "alice\x00tan", false},
		{"escape", "alice\x1btan", false},
		{"unicode letter", "ünïcode", false},
		{"unicode combining", "alicé", false},
		{"fullwidth digit", "１", false},
		{"glob star", "ali*", false},
		{"bracket", "x[1]", false},
		{"question mark", "a?b", false},
		{"trailing newline", "alice\n", false},
		{"leading space", " alice", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidSlug(tc.in); got != tc.want {
				t.Fatalf("ValidSlug(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

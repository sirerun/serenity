package redact

import (
	"strings"
	"testing"
)

// TestNewPatternValidation: a configured pattern needs a placeholder-safe
// name and a compilable regex; both failures name the offending pattern
// so config.Load can surface them verbatim.
func TestNewPatternValidation(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		expr    string
		wantErr string
	}{
		{"valid", "employee_id", `EMP-[0-9]{6}`, ""},
		{"empty name", "", `EMP-[0-9]{6}`, "name is required"},
		{"name with spaces", "employee id", `EMP-[0-9]{6}`, "employee id"},
		{"invalid regex", "broken_rule", `(`, "broken_rule"},
		{"empty regex", "empty_rule", ``, "empty_rule"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewPattern(tt.pattern, tt.expr)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("NewPattern(%q, %q): %v", tt.pattern, tt.expr, err)
				}
				if p.Name != tt.pattern {
					t.Fatalf("Name = %q, want %q", p.Name, tt.pattern)
				}
				return
			}
			if err == nil {
				t.Fatalf("NewPattern(%q, %q) accepted an invalid pattern", tt.pattern, tt.expr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("NewPattern error %q does not mention %q", err, tt.wantErr)
			}
		})
	}
}

// TestRedactApplyConfiguredPatternsExtendBuiltins: Options.Patterns adds
// named rules with their own placeholder; the built-in table still runs
// in full, so a configured rule can never disable a built-in and a
// built-in match is never re-labelled by a later configured rule.
func TestRedactApplyConfiguredPatternsExtendBuiltins(t *testing.T) {
	employeeID, err := NewPattern("employee_id", `EMP-[0-9]{6}`)
	if err != nil {
		t.Fatal(err)
	}
	// A configured rule that would also match a built-in key shape: the
	// built-in wins because it runs first, proving extras extend, never
	// override.
	greedyKey, err := NewPattern("greedy", `AKIA[A-Z0-9]+`)
	if err != nil {
		t.Fatal(err)
	}

	in := "Badge EMP-123456, key AKIATEST" + strings.Repeat("0", 12) + ", card 4111 1111 1111 1111."
	got := Apply(in, Options{Patterns: []Pattern{employeeID, greedyKey}})
	want := "Badge [REDACTED:EMPLOYEE_ID], key [REDACTED:API_KEY], card [REDACTED:CARD_NUMBER]."
	if got != want {
		t.Fatalf("Apply with configured patterns\n got: %q\nwant: %q", got, want)
	}

	if got := Apply(in, Options{}); strings.Contains(got, "[REDACTED:EMPLOYEE_ID]") {
		t.Fatalf("Apply without configured patterns emitted a configured placeholder: %q", got)
	}
}

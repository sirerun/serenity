package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/redact"
)

func writeConfigFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestLoadRedactPatternsCompileAndExtendBuiltins covers ADR 021's config
// key: `redact.patterns` is a list of named regexes that Load accepts,
// Compile turns into redact.Pattern values, and Apply honours alongside
// (never instead of) the built-in table.
func TestLoadRedactPatternsCompileAndExtendBuiltins(t *testing.T) {
	path := writeConfigFile(t, `version: 1
redact:
  patterns:
    - name: employee_id
      regex: 'EMP-[0-9]{6}'
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Redact.Patterns) != 1 || cfg.Redact.Patterns[0].Name != "employee_id" {
		t.Fatalf("Redact.Patterns = %+v, want one pattern named employee_id", cfg.Redact.Patterns)
	}

	patterns, err := cfg.Redact.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(patterns) != 1 {
		t.Fatalf("Compile returned %d patterns, want 1", len(patterns))
	}

	in := "Badge EMP-123456 and key AKIATEST" + strings.Repeat("0", 12)
	got := redact.Apply(in, redact.Options{Patterns: patterns})
	want := "Badge [REDACTED:EMPLOYEE_ID] and key [REDACTED:API_KEY]"
	if got != want {
		t.Fatalf("Apply with configured patterns\n got: %q\nwant: %q", got, want)
	}
}

// TestLoadRejectsInvalidRedactPatternRegexWithName proves an invalid
// regex fails Load (not a later call) and the error names the pattern
// so the operator can find it in serenity.yml.
func TestLoadRejectsInvalidRedactPatternRegexWithName(t *testing.T) {
	path := writeConfigFile(t, `version: 1
redact:
  patterns:
    - name: employee_id
      regex: 'EMP-[0-9]{6}'
    - name: broken_rule
      regex: '('
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load accepted an invalid redact.patterns regex")
	}
	if !strings.Contains(err.Error(), "broken_rule") {
		t.Fatalf("Load error does not name the invalid pattern: %v", err)
	}
	if !strings.Contains(err.Error(), "redact.patterns") {
		t.Fatalf("Load error does not name the config key: %v", err)
	}
}

// TestLoadRejectsRedactPatternWithoutName: a pattern with no name has no
// placeholder to emit, so it is a config error at Load.
func TestLoadRejectsRedactPatternWithoutName(t *testing.T) {
	path := writeConfigFile(t, `version: 1
redact:
  patterns:
    - regex: 'EMP-[0-9]{6}'
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("Load accepted a redact.patterns entry without a name")
	}
	if !strings.Contains(err.Error(), "redact.patterns") {
		t.Fatalf("Load error does not name the config key: %v", err)
	}
}

// TestRedactPatternsRoundTripThroughSave proves the key survives
// Save/Load unchanged and that a config without the key stays empty.
func TestRedactPatternsRoundTripThroughSave(t *testing.T) {
	cfg := Default()
	cfg.Redact.Patterns = []RedactPattern{{Name: "employee_id", Regex: `EMP-[0-9]{6}`}}
	path := filepath.Join(t.TempDir(), FileName)
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Redact.Patterns) != 1 || loaded.Redact.Patterns[0] != cfg.Redact.Patterns[0] {
		t.Fatalf("round trip lost redact.patterns: %+v", loaded.Redact.Patterns)
	}

	if patterns, err := Default().Redact.Compile(); err != nil || len(patterns) != 0 {
		t.Fatalf("Default().Redact.Compile() = %v, %v; want no patterns, nil", patterns, err)
	}
}

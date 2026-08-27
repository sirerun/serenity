// Package docs is the file-first CI gate for docs/threat-model.md: the
// document is data (like the brain repo itself), and this test suite is
// what keeps it honest against RFC 0001 §14 rather than trusting convention.
// If the required adversaries, contracts, or invariants drift out of the
// doc, this build fails.
package docs

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// threatModelPath is relative to this package directory (go test's working
// directory), two levels up from internal/docs to the repo root.
const threatModelPath = "../../docs/threat-model.md"

func readThreatModel(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(threatModelPath)
	if err != nil {
		t.Fatalf("docs/threat-model.md must exist and be readable: %v", err)
	}
	if strings.TrimSpace(string(b)) == "" {
		t.Fatal("docs/threat-model.md exists but is empty")
	}
	return string(b)
}

// headingRE matches a markdown ATX heading line ("#" through "######") and
// captures its trimmed text.
var headingRE = regexp.MustCompile(`(?m)^#{1,6}[ \t]+(.+?)[ \t]*$`)

// headings extracts every ATX heading's text from content.
func headings(content string) []string {
	matches := headingRE.FindAllStringSubmatch(content, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

// hasHeadingContaining reports whether some heading in hs contains substr,
// case-insensitively. A hit in body prose does NOT count -- callers must
// pass headings extracted by headings(), not raw file content -- because
// the point of this check is that the doc organizes the required content as
// a first-class, scannable section, not a passing mention.
func hasHeadingContaining(hs []string, substr string) bool {
	substr = strings.ToLower(substr)
	for _, h := range hs {
		if strings.Contains(strings.ToLower(h), substr) {
			return true
		}
	}
	return false
}

// TestHasHeadingContainingDetectsAbsence proves the detector used by
// TestThreatModelRequiredHeadings is not vacuously true: it must return
// false for a substring that is genuinely absent from every heading, and it
// must not be fooled by the same substring appearing only in body prose.
func TestHasHeadingContainingDetectsAbsence(t *testing.T) {
	hs := headings("# Threat model\n\n## Adversaries\n\n### Adversary 1: malicious source content\n")

	cases := []struct {
		name   string
		substr string
		want   bool
	}{
		{"top-level heading present", "adversaries", true},
		{"nested heading present, case-insensitive", "Malicious Source Content", true},
		{"substring absent from every heading", "redaction contract", false},
		{"substring absent, different wording", "right-to-forget", false},
	}
	for _, tt := range cases {
		if got := hasHeadingContaining(hs, tt.substr); got != tt.want {
			t.Errorf("hasHeadingContaining(%q) = %v, want %v", tt.substr, got, tt.want)
		}
	}

	// Negative control: a phrase that appears only in body prose, never as
	// a heading, must not be detected.
	bodyOnly := headings("# Threat model\n\nSee the redaction contract below for details.\n")
	if hasHeadingContaining(bodyOnly, "redaction contract") {
		t.Fatal("hasHeadingContaining must not match body prose, only heading text")
	}
}

// TestThreatModelRequiredHeadings asserts the doc carries a dedicated
// heading for every element the M0 residuals plan (T0.1) requires: the five
// RFC §14 adversaries, the redaction contract, and the right-to-forget
// deletion chain.
func TestThreatModelRequiredHeadings(t *testing.T) {
	hs := headings(readThreatModel(t))

	// RFC 0001 §14: "malicious or instruction-injected source content ...;
	// a malicious or compromised MCP client ...; model-provider data
	// handling; secrets accidentally present in ingested repos; local
	// attackers reading the index or key material."
	adversaries := []string{
		"instruction-injected source content",
		"compromised mcp client",
		"model-provider data handling",
		"secrets accidentally present in ingested repos",
		"local attacker on the index or key material",
	}
	for _, want := range adversaries {
		if !hasHeadingContaining(hs, want) {
			t.Errorf("missing a heading for RFC §14 adversary %q", want)
		}
	}

	required := map[string]string{
		"redaction contract":             "redaction contract",
		"right-to-forget deletion chain": "right-to-forget",
	}
	for name, substr := range required {
		if !hasHeadingContaining(hs, substr) {
			t.Errorf("missing heading for %s (want a heading containing %q)", name, substr)
		}
	}
}

// TestThreatModelPreceptInvariantLine asserts the doc states the exact
// precept-integrity invariant from RFC §7.3/§14, verbatim, not paraphrased.
func TestThreatModelPreceptInvariantLine(t *testing.T) {
	content := readThreatModel(t)
	const want = "no ingest path can create or modify a precept"
	if !strings.Contains(content, want) {
		t.Fatalf("docs/threat-model.md must contain the exact line %q", want)
	}
}

// TestThreatModelSingleMermaidDiagram enforces the T0.1 pitfall: exactly one
// mermaid block, and no second diagram format competing with it.
func TestThreatModelSingleMermaidDiagram(t *testing.T) {
	content := readThreatModel(t)

	if got := strings.Count(content, "```mermaid"); got != 1 {
		t.Errorf("want exactly one ```mermaid block, got %d", got)
	}

	for _, other := range []string{"```plantuml", "```dot", "```graphviz", "```puml", "```svg"} {
		if strings.Contains(content, other) {
			t.Errorf("found a second diagram format %q alongside the mermaid block; RFC 0001 requires exactly one diagram", other)
		}
	}
}

// TestThreatModelCoversDataFlowAndKeyMaterial asserts the remaining T0.1
// content requirements that are prose-level rather than heading-level: the
// data-flow framing, the keys-in-keychain rule, and the loopback-auth
// daemon rule.
func TestThreatModelCoversDataFlowAndKeyMaterial(t *testing.T) {
	content := readThreatModel(t)

	mustContain := []string{
		// Data-flow diagram section (RFC §14: "data-flow diagram required
		// in docs").
		"what leaves the machine",
		// Keys-in-keychain (RFC §14: "model API keys and connector OAuth
		// tokens live in the OS keychain, never in files").
		"OS keychain",
		// Loopback-authenticated daemon (RFC §14: "binds localhost with a
		// bearer token required by default (loopback is authenticated
		// too...)").
		"bearer token",
		// Right-to-forget deletion chain order (RFC §14).
		"source",
		"observations",
		"claims",
		"rebuild",
	}
	for _, want := range mustContain {
		if !strings.Contains(content, want) {
			t.Errorf("docs/threat-model.md is missing required content: %q", want)
		}
	}
}

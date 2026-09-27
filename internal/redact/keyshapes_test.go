package redact

import (
	"strings"
	"testing"
)

// syntheticKeyFixture is the table of real-shaped, obviously fake key
// values deep review 001 (AI-02) probed: the nine modern vendor shapes
// the legacy regex let through, Slack's three token prefixes counted
// separately, and the two legacy shapes (bare sk-<alnum>, AKIA) that
// must keep working. Every value is built from a TEST marker and a
// zero run so no secret scanner mistakes it for a live credential.
func syntheticKeyFixture() []struct{ name, key string } {
	zeros := func(n int) string { return strings.Repeat("0", n) }
	return []struct{ name, key string }{
		{"anthropic sk-ant-", "sk-ant-api03-TEST" + zeros(74) + "AA"},
		{"openai project sk-proj-", "sk-proj-TEST" + zeros(44) + "T3BlbkFJTEST" + zeros(40)},
		{"openai service account sk-svcacct-", "sk-svcacct-TEST" + zeros(60)},
		{"openrouter sk-or-", "sk-or-v1-" + zeros(64)},
		{"github ghp_", "ghp_TEST" + zeros(32)},
		{"slack bot xoxb-", "xoxb-" + zeros(12) + "-" + zeros(12) + "-TEST" + zeros(20)},
		{"slack user xoxp-", "xoxp-" + zeros(12) + "-" + zeros(12) + "-" + zeros(12) + "-TEST" + zeros(28)},
		{"slack app xoxa-", "xoxa-2-" + zeros(12) + "-" + zeros(12) + "-TEST" + zeros(22)},
		{"google AIza", "AIzaTEST" + zeros(31)},
		{"stripe secret sk_live_", "sk_live_TEST" + zeros(24)},
		{"stripe restricted rk_live_", "rk_live_TEST" + zeros(24)},
		{"legacy openai sk-", "sk-TEST" + zeros(44)},
		{"legacy aws AKIA", "AKIATEST" + zeros(12)},
	}
}

// TestRedactApplyModernKeyShapeFixture is T24.12's fixture: every
// synthetic key shape named in deep review 001 must be replaced by the
// API_KEY placeholder in full, whether it stands alone or sits inside a
// sentence, and no fragment of any value may survive in the output.
func TestRedactApplyModernKeyShapeFixture(t *testing.T) {
	for _, tc := range syntheticKeyFixture() {
		t.Run(tc.name, func(t *testing.T) {
			in := "token: " + tc.key + " (rotate it)"
			got := Apply(in, Options{})
			want := "token: [REDACTED:API_KEY] (rotate it)"
			if got != want {
				t.Fatalf("Apply(%q)\n got: %q\nwant: %q", in, got, want)
			}
			if strings.Contains(got, tc.key[len(tc.key)-8:]) {
				t.Fatalf("Apply leaked a fragment of %s: %q", tc.name, got)
			}
		})
	}
}

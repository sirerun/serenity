package redact

import (
	"strings"
	"testing"
)

// TestRedactApplyCardNumberLuhnGate proves card-number detection is
// Luhn-gated, not shape-only: a Luhn-valid 16-digit run (grouped with
// spaces) is redacted; a same-length run that fails the Luhn checksum
// is left untouched.
func TestRedactApplyCardNumberLuhnGate(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "luhn valid card is redacted",
			in:   "Card on file: 4111 1111 1111 1111.",
			want: "Card on file: [REDACTED:CARD_NUMBER].",
		},
		{
			name: "luhn invalid lookalike is untouched",
			in:   "Reference code 1234567890123456 does not match any card.",
			want: "Reference code 1234567890123456 does not match any card.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Apply(tt.in, Options{})
			if got != tt.want {
				t.Fatalf("Apply(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestRedactApplyAccountNumberKeywordGate proves account-number
// detection is keyword-gated: a digit run near an account keyword is
// redacted, the identical digit run without a keyword nearby is not,
// and a Luhn-valid card number near the word "account" is redacted as
// a card, never as an account number.
func TestRedactApplyAccountNumberKeywordGate(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "keyword-preceded digits are redacted as account",
			in:   "Please wire funds to account number 987654321012 for processing.",
			want: "Please wire funds to account number [REDACTED:ACCOUNT_NUMBER] for processing.",
		},
		{
			name: "same digits with no keyword nearby are untouched",
			in:   "Order confirmation 987654321012 shipped today.",
			want: "Order confirmation 987654321012 shipped today.",
		},
		{
			name: "card number near account keyword stays a card",
			in:   "account 4111 1111 1111 1111",
			want: "account [REDACTED:CARD_NUMBER]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Apply(tt.in, Options{})
			if got != tt.want {
				t.Fatalf("Apply(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestRedactApplyAPIKeyShapes proves API-key shape detection covers
// two structurally distinct vendor shapes (sk- opaque tokens, AKIA
// AWS access key ids) and leaves a short non-matching token alone.
func TestRedactApplyAPIKeyShapes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "sk- prefixed token is redacted",
			in:   "API token: sk-TESTFAKEKEY1234567890ABCDEFGHIJKL",
			want: "API token: [REDACTED:API_KEY]",
		},
		{
			name: "AKIA prefixed token is redacted",
			in:   "API token: AKIAFAKEEXAMPLE12345",
			want: "API token: [REDACTED:API_KEY]",
		},
		{
			name: "short non-matching token is untouched",
			in:   "Not a key: id-12345",
			want: "Not a key: id-12345",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Apply(tt.in, Options{})
			if got != tt.want {
				t.Fatalf("Apply(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestRedactApplyEmailOnRequest proves emails are redacted only when
// explicitly requested, and left untouched by the zero-value Options.
func TestRedactApplyEmailOnRequest(t *testing.T) {
	in := "Contact jane.doe@example.com for details."

	got := Apply(in, Options{RedactEmails: true})
	want := "Contact [REDACTED:EMAIL] for details."
	if got != want {
		t.Fatalf("Apply(%q, RedactEmails=true) = %q, want %q", in, got, want)
	}

	got = Apply(in, Options{})
	if got != in {
		t.Fatalf("Apply(%q, Options{}) = %q, want unchanged %q", in, got, in)
	}
}

// TestRedactApplyGoldenCorpus pins the exact output of a single
// fixture corpus exercising every pattern together, and asserts none
// of the original sensitive substrings survives anywhere in the
// output (the anti-partial-leak check: no placeholder may retain any
// fragment, e.g. a last-4-digit reveal, of the value it replaced).
func TestRedactApplyGoldenCorpus(t *testing.T) {
	in := strings.Join([]string{
		"Please wire funds to account number 987654321012 for processing.",
		"Order confirmation 987654321012 shipped today.",
		"Card on file: 4111 1111 1111 1111.",
		"Reference code 1234567890123456 does not match any card.",
		"API token: sk-TESTFAKEKEY1234567890ABCDEFGHIJKL",
		"API token: AKIAFAKEEXAMPLE12345",
		"Not a key: id-12345",
		"Contact jane.doe@example.com for details.",
	}, "\n")

	want := strings.Join([]string{
		"Please wire funds to account number [REDACTED:ACCOUNT_NUMBER] for processing.",
		"Order confirmation 987654321012 shipped today.",
		"Card on file: [REDACTED:CARD_NUMBER].",
		"Reference code 1234567890123456 does not match any card.",
		"API token: [REDACTED:API_KEY]",
		"API token: [REDACTED:API_KEY]",
		"Not a key: id-12345",
		"Contact [REDACTED:EMAIL] for details.",
	}, "\n")

	got := Apply(in, Options{RedactEmails: true})
	if got != want {
		t.Fatalf("Apply golden corpus mismatch:\ngot:\n%s\nwant:\n%s", got, want)
	}

	// 987654321012 is deliberately excluded here: the corpus's second
	// line is the negative-space fixture proving a bare digit run with
	// no account keyword nearby stays unredacted, so that exact value
	// is expected to survive once in the output. Every other sensitive
	// value in the corpus is redacted on every occurrence, so none of
	// them may survive anywhere.
	leaked := []string{
		"4111 1111 1111 1111",
		"sk-TESTFAKEKEY1234567890ABCDEFGHIJKL",
		"AKIAFAKEEXAMPLE12345",
		"jane.doe@example.com",
	}
	for _, v := range leaked {
		if strings.Contains(got, v) {
			t.Fatalf("Apply output leaked original sensitive value %q:\n%s", v, got)
		}
	}
}

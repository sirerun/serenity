// Package redact detects PII- and secret-shaped patterns (account
// numbers, card numbers, API-key shapes, and — on request — email
// addresses) and replaces each match with a typed placeholder. RFC
// 0001 §14 requires a redaction pass to run on every prompt before it
// leaves the machine for a cloud model; this package is that pass.
//
// Patterns are applied in a fixed order so spans never double-match:
// API-key shapes first, then card numbers (Luhn-gated), then account
// numbers (keyword-gated), then emails (only when Options.RedactEmails
// is set). A placeholder always covers the entire matched span — it
// never retains any fragment of the original value, e.g. a last-4
// digit reveal.
package redact

import (
	"regexp"
	"strings"
)

// PlaceholderType names the category a redacted span belonged to.
type PlaceholderType string

const (
	PlaceholderAccountNumber PlaceholderType = "ACCOUNT_NUMBER"
	PlaceholderCardNumber    PlaceholderType = "CARD_NUMBER"
	PlaceholderAPIKey        PlaceholderType = "API_KEY"
	PlaceholderEmail         PlaceholderType = "EMAIL"
)

// Options controls which optional patterns Apply enables. Account
// numbers, card numbers, and API-key shapes are always redacted;
// emails are redacted only when explicitly requested (RFC 0001 §14
// names email as the on-request pattern).
type Options struct {
	RedactEmails bool
}

// accountKeywords gate account-number detection so a bare long digit
// run is never redacted on shape alone (§14: patterns + entity-type
// rules, not a blanket digit filter).
var accountKeywords = []string{"account", "acct", "routing", "iban"}

// accountKeywordWindow is how many characters immediately before a
// candidate digit run are searched for an account keyword.
const accountKeywordWindow = 24

var (
	// API-key shapes: an sk-prefixed opaque token (Anthropic/OpenAI-
	// style secret keys) or an AKIA-prefixed AWS access key id.
	apiKeyPattern = regexp.MustCompile(`\bsk-[A-Za-z0-9]{20,}\b|\bAKIA[A-Z0-9]{16}\b`)

	// A candidate digit run: a digit followed by any number of
	// (single-space-or-hyphen, digit) groups. Whether it becomes a
	// CARD_NUMBER, ACCOUNT_NUMBER, or is left alone is decided per
	// match in redactDigitRuns.
	digitRunPattern = regexp.MustCompile(`\d(?:[-\s]?\d)*`)

	emailPattern = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
)

func placeholder(t PlaceholderType) string {
	return "[REDACTED:" + string(t) + "]"
}

// Apply returns text with every seeded pattern replaced by a typed
// placeholder. It never returns any substring of the original matched
// value.
func Apply(text string, opts Options) string {
	text = apiKeyPattern.ReplaceAllString(text, placeholder(PlaceholderAPIKey))
	text = redactDigitRuns(text)
	if opts.RedactEmails {
		text = emailPattern.ReplaceAllString(text, placeholder(PlaceholderEmail))
	}
	return text
}

// redactDigitRuns classifies each candidate digit run as a card
// number (13-19 digits, Luhn-valid), an account number (8-17 digits,
// preceded by an account keyword), or leaves it untouched.
func redactDigitRuns(text string) string {
	matches := digitRunPattern.FindAllStringIndex(text, -1)
	if matches == nil {
		return text
	}
	var b strings.Builder
	last := 0
	for _, m := range matches {
		start, end := m[0], m[1]
		span := text[start:end]
		digits := onlyDigits(span)
		b.WriteString(text[last:start])
		switch {
		case len(digits) >= 13 && len(digits) <= 19 && luhnValid(digits):
			b.WriteString(placeholder(PlaceholderCardNumber))
		case len(digits) >= 8 && len(digits) <= 17 && precededByAccountKeyword(text, start):
			b.WriteString(placeholder(PlaceholderAccountNumber))
		default:
			b.WriteString(span)
		}
		last = end
	}
	b.WriteString(text[last:])
	return b.String()
}

func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// precededByAccountKeyword reports whether one of accountKeywords
// appears, case-insensitively, in the accountKeywordWindow characters
// immediately before matchStart.
func precededByAccountKeyword(text string, matchStart int) bool {
	windowStart := matchStart - accountKeywordWindow
	if windowStart < 0 {
		windowStart = 0
	}
	window := strings.ToLower(text[windowStart:matchStart])
	for _, kw := range accountKeywords {
		if strings.Contains(window, kw) {
			return true
		}
	}
	return false
}

// luhnValid reports whether digits (ASCII '0'-'9' only) passes the
// Luhn checksum used by card numbers.
func luhnValid(digits string) bool {
	sum := 0
	double := false
	for i := len(digits) - 1; i >= 0; i-- {
		d := int(digits[i] - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}

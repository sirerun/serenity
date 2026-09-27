// Package redact detects PII- and secret-shaped patterns (account
// numbers, card numbers, API-key shapes, and — on request — email
// addresses) and replaces each match with a typed placeholder. RFC
// 0001 §14 requires a redaction pass to run on every prompt before it
// leaves the machine for a cloud model; this package is that pass, and
// internal/router.Router.Complete is the one place it runs (ADR 021):
// every Complete and Embed call on every provider passes through it.
//
// Patterns are applied in a fixed order so spans never double-match:
// the built-in API-key table first, then card numbers (Luhn-gated),
// then account numbers (keyword-gated), then emails (only when
// Options.RedactEmails is set), then operator-configured patterns
// (Options.Patterns, serenity.yml `redact.patterns`). A placeholder
// always covers the entire matched span — it never retains any
// fragment of the original value, e.g. a last-4 digit reveal.
package redact

import (
	"errors"
	"fmt"
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
// names email as the on-request pattern). Patterns extends the
// built-in set and can never remove any part of it.
type Options struct {
	RedactEmails bool
	// Patterns are operator-configured rules (serenity.yml
	// `redact.patterns`, ADR 021), applied after every built-in rule.
	// Build them with NewPattern; the zero Pattern matches nothing.
	Patterns []Pattern
}

// Pattern is one named, operator-supplied regex. A match is replaced by
// "[REDACTED:<NAME>]" with Name upper-cased. Construct with NewPattern
// so the name and regex are validated once, at config load, rather than
// on the egress path.
type Pattern struct {
	Name string
	re   *regexp.Regexp
}

// patternNameExpr keeps configured names placeholder-safe: a placeholder
// is "[REDACTED:<NAME>]", so the name may not contain whitespace or
// brackets.
const patternNameExpr = `^[A-Za-z][A-Za-z0-9_-]*$`

var patternNamePattern = regexp.MustCompile(patternNameExpr)

// NewPattern validates and compiles one configured rule. Every error
// names the pattern (or says the name is missing) so config.Load can
// surface it verbatim.
func NewPattern(name, expr string) (Pattern, error) {
	if name == "" {
		return Pattern{}, errors.New("redact: pattern name is required")
	}
	if !patternNamePattern.MatchString(name) {
		return Pattern{}, fmt.Errorf("redact: pattern %q: name must match %s", name, patternNameExpr)
	}
	if expr == "" {
		return Pattern{}, fmt.Errorf("redact: pattern %q: regex is required", name)
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return Pattern{}, fmt.Errorf("redact: pattern %q: %w", name, err)
	}
	return Pattern{Name: name, re: re}, nil
}

func (p Pattern) placeholder() string {
	return placeholder(PlaceholderType(strings.ToUpper(p.Name)))
}

// keyShape is one row of the built-in API-key table.
type keyShape struct {
	vendor string
	re     *regexp.Regexp
}

// apiKeyShapes is the built-in API-key table (deep review 001, AI-02).
// Order matters: the vendor-prefixed sk- shapes precede the legacy bare
// sk-<alnum> shape so a modern key is matched whole. Each row is a
// real-shaped prefix plus a bounded body; every row is exercised by the
// synthetic fixture in keyshapes_test.go. Nothing in Options can remove
// a row.
var apiKeyShapes = []keyShape{
	{"anthropic", regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}`)},
	{"openai project", regexp.MustCompile(`\bsk-proj-[A-Za-z0-9_-]{20,}`)},
	{"openai service account", regexp.MustCompile(`\bsk-svcacct-[A-Za-z0-9_-]{20,}`)},
	{"openrouter", regexp.MustCompile(`\bsk-or-[A-Za-z0-9_-]{20,}`)},
	{"openai legacy", regexp.MustCompile(`\bsk-[A-Za-z0-9]{20,}\b`)},
	{"github token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}\b`)},
	{"github fine-grained token", regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{22,}\b`)},
	{"slack token", regexp.MustCompile(`\bxox[abpsr]-[A-Za-z0-9-]{10,}`)},
	{"google api key", regexp.MustCompile(`\bAIza[A-Za-z0-9_-]{35}\b`)},
	{"stripe secret or restricted key", regexp.MustCompile(`\b[sr]k_(?:live|test)_[A-Za-z0-9]{16,}\b`)},
	{"aws access key id", regexp.MustCompile(`\bAKIA[A-Z0-9]{16}\b`)},
}

// accountKeywords gate account-number detection so a bare long digit
// run is never redacted on shape alone (§14: patterns + entity-type
// rules, not a blanket digit filter).
var accountKeywords = []string{"account", "acct", "routing", "iban"}

// accountKeywordWindow is how many characters immediately before a
// candidate digit run are searched for an account keyword.
const accountKeywordWindow = 24

var (
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
// value. Built-in rules run first, in full, regardless of opts;
// opts.Patterns run last and can only add placeholders.
func Apply(text string, opts Options) string {
	for _, shape := range apiKeyShapes {
		text = shape.re.ReplaceAllString(text, placeholder(PlaceholderAPIKey))
	}
	text = redactDigitRuns(text)
	if opts.RedactEmails {
		text = emailPattern.ReplaceAllString(text, placeholder(PlaceholderEmail))
	}
	for _, p := range opts.Patterns {
		if p.re == nil {
			continue
		}
		text = p.re.ReplaceAllString(text, p.placeholder())
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

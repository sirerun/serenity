// Package neutralize removes terminal control content from untrusted text
// before it is printed or returned to a client (ADR 022, decision 3).
//
// Ingested source text and model output can carry ANSI escape sequences
// and control characters that rewrite an operator's terminal: conceal a
// payload, erase and redraw the line being reviewed, plant a hyperlink or
// retitle the window (deep review 001, SEC-M05). Text is the single helper
// every CLI print of such text and the MCP synthesize answer pass through.
package neutralize

import (
	"strings"
	"unicode/utf8"
)

const (
	esc = 0x1b
	bel = 0x07

	// C1 introducers and terminator, as runes (U+0080-U+009F).
	c1DCS = 0x90
	c1SOS = 0x98
	c1CSI = 0x9b
	c1ST  = 0x9c
	c1OSC = 0x9d
	c1PM  = 0x9e
	c1APC = 0x9f
)

// Text returns s with every ESC sequence (CSI, OSC, DCS, SOS, PM, APC and
// two-byte escapes), every C0 control except newline and tab, DEL, and
// every C1 control removed. A control string (OSC, DCS, SOS, PM, APC) is
// removed through its BEL or ST terminator; an unterminated one is removed
// to the end of s, since none of its content was meant to be displayed.
// Invalid UTF-8 bytes become U+FFFD so a raw 8-bit C1 byte cannot reach a
// terminal that is not in UTF-8 mode. Text is idempotent.
func Text(s string) string {
	if clean(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			b.WriteRune(utf8.RuneError)
			i++
		case r == esc:
			i = skipEscape(s, i+size)
		case r == c1CSI:
			i = skipCSI(s, i+size)
		case r == c1OSC || r == c1DCS || r == c1SOS || r == c1PM || r == c1APC:
			i = skipString(s, i+size)
		case isControl(r):
			i += size
		default:
			b.WriteString(s[i : i+size])
			i += size
		}
	}
	return b.String()
}

// clean reports whether s needs no rewriting: valid UTF-8 with no control
// characters other than newline and tab.
func clean(s string) bool {
	for _, r := range s {
		if r == utf8.RuneError || isControl(r) {
			return false
		}
	}
	return true
}

// isControl reports whether r is a C0 control other than newline or tab,
// DEL, or a C1 control.
func isControl(r rune) bool {
	return (r < 0x20 && r != '\n' && r != '\t') || (r >= 0x7f && r < 0xa0)
}

// skipEscape consumes the sequence following an ESC at s[i-1] and returns
// the index just past it.
func skipEscape(s string, i int) int {
	if i >= len(s) {
		return i
	}
	switch c := s[i]; {
	case c == '[':
		return skipCSI(s, i+1)
	case c == ']' || c == 'P' || c == 'X' || c == '^' || c == '_':
		return skipString(s, i+1)
	case c >= 0x20 && c <= 0x2f:
		// nF escape: intermediate bytes, then one final byte.
		for i < len(s) && s[i] >= 0x20 && s[i] <= 0x2f {
			i++
		}
		if i < len(s) {
			_, size := utf8.DecodeRuneInString(s[i:])
			i += size
		}
		return i
	default:
		// Two-byte escape (Fp, Fe, Fs): drop the one following character.
		_, size := utf8.DecodeRuneInString(s[i:])
		return i + size
	}
}

// skipCSI consumes a control sequence's parameter and intermediate bytes
// (0x20-0x3F) and its final byte (0x40-0x7E). Any other byte ends the
// sequence without being consumed.
func skipCSI(s string, i int) int {
	for i < len(s) && s[i] >= 0x20 && s[i] <= 0x3f {
		i++
	}
	if i < len(s) && s[i] >= 0x40 && s[i] <= 0x7e {
		i++
	}
	return i
}

// skipString consumes a control string through its terminator: BEL, ESC \
// or the C1 ST rune. Without a terminator it consumes the rest of s.
func skipString(s string, i int) int {
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == bel || r == c1ST:
			return i + size
		case r == esc && i+1 < len(s) && s[i+1] == '\\':
			return i + 2
		}
		i += size
	}
	return i
}

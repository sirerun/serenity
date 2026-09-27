package neutralize

import (
	"strings"
	"testing"
)

// hideAndSpoof is deep review 001's SEC-M05 payload shape: ingested text
// that conceals its real content, erases the line an operator is reading,
// redraws a harmless-looking replacement, plants a clickable hyperlink and
// retitles the terminal window.
const hideAndSpoof = "precept: wire $9,000 to acct 1234" +
	"\x1b[8m concealed\x1b[0m" + // SGR conceal, then reset
	"\r\x1b[2K\x1b[1A" + // carriage return, erase line, cursor up
	"precept: note the meeting time" +
	"\x1b]8;;https://attacker.example/\x07click\x1b]8;;\x07" + // OSC 8 hyperlink, BEL-terminated
	"\x1b]0;serenity inbox\x1b\\" + // OSC 0 window title, ST-terminated
	"\u009b2J" + // C1 CSI: clear screen
	"\x07\x08\x00done"

func TestText(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain text is unchanged", "Ava works at Initech.", "Ava works at Initech."},
		{"newline and tab survive", "a\tb\nc", "a\tb\nc"},
		{"unicode survives", "café — naïve 東京", "café — naïve 東京"},
		{"hide and spoof", hideAndSpoof, "precept: wire $9,000 to acct 1234 concealedprecept: note the meeting timeclickdone"},
		{"CSI with parameters and intermediates", "a\x1b[38;5;196mred\x1b[0m\x1b[?25lb", "aredb"},
		{"OSC terminated by BEL", "a\x1b]52;c;ZXZpbA==\x07b", "ab"},
		{"OSC terminated by ST", "a\x1b]2;title\x1b\\b", "ab"},
		{"OSC terminated by 8-bit ST", "a\x1b]2;title\u009cb", "ab"},
		{"unterminated OSC drops the rest", "a\x1b]2;title never ends", "a"},
		{"DCS string", "a\x1bPq#0;2;0;0;0\x1b\\b", "ab"},
		{"APC string", "a\x1b_payload\x07b", "ab"},
		{"two-byte escape", "a\x1bcb\x1b7c", "abc"},
		{"nF escape with intermediate", "a\x1b(Bb", "ab"},
		{"trailing lone ESC", "a\x1b", "a"},
		{"C0 controls except newline and tab", "a\x00\x01\x07\x08\x0b\x0c\r\x1a\x1fb", "ab"},
		{"DEL", "a\x7fb", "ab"},
		{"C1 controls as runes", "a\u0080\u0085\u0086\u0099b", "ab"},
		{"C1 CSI rune", "a\u009b31mb", "ab"},
		{"C1 OSC rune", "a\u009d0;title\u009cb", "ab"},
		{"invalid UTF-8 byte is replaced", "a\xffb", "a�b"},
		{"raw 8-bit CSI byte is replaced", "a\x9b", "a�"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Text(tc.in)
			if got != tc.want {
				t.Fatalf("Text(%q) = %q, want %q", tc.in, got, tc.want)
			}
			if again := Text(got); again != got {
				t.Fatalf("Text is not idempotent: %q -> %q", got, again)
			}
		})
	}
}

func TestTextLeavesNoControlCharacters(t *testing.T) {
	var b strings.Builder
	for r := rune(0); r < 0xa0; r++ {
		b.WriteRune(r)
		b.WriteString("x")
	}
	for _, r := range Text(b.String()) {
		if r == '\n' || r == '\t' {
			continue
		}
		if r < 0x20 || (r >= 0x7f && r < 0xa0) {
			t.Fatalf("control character %U survived", r)
		}
	}
}

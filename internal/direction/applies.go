// Package direction implements Serenity's own constraint layer over a dira
// ledger (RFC 0001 §7.3, §8.3; ADR 008): the applies_when clause a
// constraint entry carries, and the disposition-only precept writer built on
// top of it.
package direction

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/sirerun/serenity/internal/dira/frontmatter"
	"github.com/sirerun/serenity/internal/domain"
)

// Fence markers for the applies_when body block ADR 008 specifies. dira's
// entry.schema.json sets additionalProperties: false and has no
// applies_when field, so the clause lives in a fenced block in the markdown
// body instead -- dira treats the body as prose it never inspects, and every
// entry Serenity writes still validates against the vendored schema
// unmodified.
const (
	appliesWhenOpenFence  = "```serenity:applies_when"
	appliesWhenCloseFence = "```"
)

// ErrMisplacedAppliesWhen marks an entry that put applies_when in YAML
// frontmatter instead of the ADR 008 body block. dira's schema would reject
// the frontmatter anyway (additionalProperties: false), but that failure
// talks about an unrecognized property; this error names the actual mistake
// so a caller can point a human straight at the fix.
var ErrMisplacedAppliesWhen = errors.New("direction: applies_when belongs in the body block, not frontmatter (ADR 008)")

// ErrUnknownAction marks an applies_when clause naming an action outside
// domain.ActionSet, the small closed action set constraints may guard
// (RFC §7.3).
var ErrUnknownAction = errors.New("direction: action is not in domain.ActionSet")

// ErrNoAppliesWhenBlock marks a body with no serenity:applies_when fence.
// Most entries -- intents, decisions, questions, notes, and constraints that
// are not yet machine-checkable -- legitimately have none; this lets a
// caller that does expect one tell "absent" from "malformed".
var ErrNoAppliesWhenBlock = errors.New("direction: no serenity:applies_when block in body")

// AppliesWhenBlock is a parsed applies_when body block: the clause it
// carries, plus the exact bytes it was read from, so it can be re-rendered
// without reformatting anything nobody asked to change.
type AppliesWhenBlock struct {
	domain.AppliesWhen

	// Raw is the fenced block's content -- everything between the opening
	// and closing fence lines, byte for byte, including the trailing
	// newline before the close fence. RenderAppliesWhenBlock never
	// re-marshals it, so a block nobody edited round-trips exactly.
	Raw []byte
}

// appliesWhenYAML is the block's on-disk shape. It exists only so
// yaml.Unmarshal has a typed target: domain.AppliesWhen is also DIRECTION's
// JSON wire type (RFC §8.3) and carries no yaml tags of its own, so this
// type keeps that struct from taking on serialization concerns for a format
// it does not own.
type appliesWhenYAML struct {
	Action string         `yaml:"action"`
	Params map[string]any `yaml:"params"`
}

// ParseAppliesWhen finds the first serenity:applies_when fenced block in an
// entry's markdown body and parses it.
//
// It returns an error wrapping ErrNoAppliesWhenBlock when body carries no
// such fence, and an error wrapping ErrUnknownAction when the block names an
// action outside domain.ActionSet.
//
// The search is byte-oriented and line-based, mirroring
// internal/dira/frontmatter.Split: CRLF is normalized to LF first, and a
// fence is a line that, trimmed of trailing spaces and tabs, equals the
// fence marker exactly -- indentation or trailing content inside the block
// does not close it early.
func ParseAppliesWhen(body []byte) (*AppliesWhenBlock, error) {
	text := strings.ReplaceAll(string(body), "\r\n", "\n")

	_, contentStart, ok := findFenceLine(text, 0, appliesWhenOpenFence)
	if !ok {
		return nil, ErrNoAppliesWhenBlock
	}

	closeLineStart, _, closeOK := findFenceLine(text, contentStart, appliesWhenCloseFence)
	if !closeOK {
		return nil, fmt.Errorf("%w: opened but never closed", ErrNoAppliesWhenBlock)
	}

	raw := []byte(text[contentStart:closeLineStart])

	var y appliesWhenYAML
	if err := yaml.Unmarshal(raw, &y); err != nil {
		return nil, fmt.Errorf("direction: parsing applies_when block: %w", err)
	}
	if y.Action == "" {
		return nil, errors.New("direction: applies_when block has no action")
	}
	if !validAction(y.Action) {
		return nil, fmt.Errorf("%w: %q", ErrUnknownAction, y.Action)
	}

	return &AppliesWhenBlock{
		AppliesWhen: domain.AppliesWhen{Action: y.Action, Params: y.Params},
		Raw:         raw,
	}, nil
}

// findFenceLine scans text from offset looking for a line, trimmed of
// trailing spaces and tabs, that equals marker exactly. It returns where
// that line begins (lineStart) and the offset immediately after its newline
// -- or end of text, for a final line with no trailing newline -- (after),
// plus whether a match was found.
func findFenceLine(text string, offset int, marker string) (lineStart, after int, ok bool) {
	for offset < len(text) {
		end := strings.IndexByte(text[offset:], '\n')
		lineEnd := len(text)
		next := len(text)
		if end >= 0 {
			lineEnd = offset + end
			next = lineEnd + 1
		}
		line := strings.TrimRight(text[offset:lineEnd], " \t")
		if line == marker {
			return offset, next, true
		}
		offset = next
	}
	return 0, 0, false
}

// RenderAppliesWhenBlock reproduces the fenced serenity:applies_when block
// exactly as ParseAppliesWhen read it. Raw is opaque content that is never
// re-marshaled, so a block nobody edited round-trips byte for byte.
func RenderAppliesWhenBlock(b *AppliesWhenBlock) []byte {
	var buf bytes.Buffer
	buf.WriteString(appliesWhenOpenFence)
	buf.WriteByte('\n')
	buf.Write(b.Raw)
	buf.WriteString(appliesWhenCloseFence)
	buf.WriteByte('\n')
	return buf.Bytes()
}

// ValidateAppliesWhenPlacement checks one dira entry file against the
// placement rule ADR 008 states: applies_when may appear only in the body
// block, never in frontmatter. Frontmatter carrying an applies_when key
// fails with an error wrapping ErrMisplacedAppliesWhen; a body block naming
// an action outside domain.ActionSet fails with an error wrapping
// ErrUnknownAction; a body with no block at all is valid (most entries have
// none) and returns nil.
//
// This does not run entry.schema.json -- callers that also want the
// vendored schema check call internal/dira/schema.Validator.Validate
// separately. The two checks are independent (the body is outside that
// schema's contract) and both are expected to pass on every entry Serenity
// writes.
func ValidateAppliesWhenPlacement(entryFile []byte) error {
	front, body, err := frontmatter.Split(entryFile)
	if err != nil {
		return err
	}

	var fm map[string]any
	if err := yaml.Unmarshal(front, &fm); err != nil {
		return fmt.Errorf("direction: parsing frontmatter: %w", err)
	}
	if _, misplaced := fm["applies_when"]; misplaced {
		return ErrMisplacedAppliesWhen
	}

	if _, err := ParseAppliesWhen(body); err != nil && !errors.Is(err, ErrNoAppliesWhenBlock) {
		return err
	}
	return nil
}

// validAction reports whether action is a member of the closed action set
// constraints may guard (RFC §7.3, domain.ActionSet).
func validAction(action string) bool {
	for _, a := range domain.ActionSet {
		if a == action {
			return true
		}
	}
	return false
}

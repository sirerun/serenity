package conformance

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// hexIDPattern matches an opaque, server-assigned identifier: a lowercase
// hex string of at least 16 characters (covers DISPOSITION's 32-hex
// crypto/rand item ids -- internal/disposition.Store.Create -- and MCP's
// own 32-hex Streamable HTTP session ids). Sixteen is a deliberate floor:
// short numeric strings a case pins deliberately (a "next_cursor":"2"
// pagination token, a "g1" group_id) never reach it, so they still compare
// literally, and only a value that genuinely could not have been
// reproduced by a second run is ever treated as dynamic.
var hexIDPattern = regexp.MustCompile(`^[0-9a-f]{16,}$`)

// timestampPattern matches an RFC 3339 timestamp (with optional fractional
// seconds), the shape every created_at/updated_at/disposed_at/occurred_at
// field in the DISPOSITION and DIRECTION transcripts uses.
var timestampPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$`)

// looksDynamic reports whether s has the SHAPE of a value T4.13's own
// disclosed gap says a replayer must not byte-compare: an opaque hex id or
// an RFC 3339 timestamp. Normalization is shape-based, not field-name-
// based, on purpose -- a curated list of "volatile field names" would
// either miss a field this task's author didn't anticipate or, worse,
// silently swallow a genuine regression in a field whose name happens to
// match id/timestamp-ish but whose value (e.g. an enum, an idempotency
// key, a group id) is not actually dynamic. A value that doesn't have this
// shape is always compared for exact equality, however it's named.
func looksDynamic(s string) bool {
	return hexIDPattern.MatchString(s) || timestampPattern.MatchString(s)
}

// Mismatch is one path where an actual response body diverged from the
// expected one in a way normalization did not excuse.
type Mismatch struct {
	Path     string
	Expected string
	Actual   string
}

func (m Mismatch) String() string {
	return fmt.Sprintf("%s: expected %s, got %s", m.Path, m.Expected, m.Actual)
}

// CompareBodies compares a transcript Step's frozen expected response body
// against a live actual response body. Both are parsed as JSON when
// possible; a body that isn't valid JSON on either side (net/http's own
// plain-text error bodies, e.g. "unauthorized\n" from an auth rejection)
// falls back to an exact string comparison, since there is no structure to
// walk and no dynamic field to excuse. Returns the mismatches found (nil
// on a match) -- never partial credit: dynamic-shaped values that match in
// TYPE (not value) are excused; everything else must be identical.
func CompareBodies(expected, actual string) []Mismatch {
	var expJSON, actJSON any
	expErr := json.Unmarshal([]byte(expected), &expJSON)
	actErr := json.Unmarshal([]byte(actual), &actJSON)
	if expErr != nil || actErr != nil {
		if expected == actual {
			return nil
		}
		return []Mismatch{{Path: "$", Expected: expected, Actual: actual}}
	}
	var mismatches []Mismatch
	diffValue("$", expJSON, actJSON, &mismatches)
	return mismatches
}

func diffValue(path string, expected, actual any, out *[]Mismatch) {
	switch exp := expected.(type) {
	case string:
		act, ok := actual.(string)
		if !ok {
			*out = append(*out, Mismatch{path, jsonify(expected), jsonify(actual)})
			return
		}
		if exp == act {
			return
		}
		if looksDynamic(exp) && looksDynamic(act) {
			return // both dynamic-shaped: excused, values need not match
		}
		*out = append(*out, Mismatch{path, jsonify(expected), jsonify(actual)})
	case map[string]any:
		act, ok := actual.(map[string]any)
		if !ok {
			*out = append(*out, Mismatch{path, jsonify(expected), jsonify(actual)})
			return
		}
		diffObject(path, exp, act, out)
	case []any:
		act, ok := actual.([]any)
		if !ok {
			*out = append(*out, Mismatch{path, jsonify(expected), jsonify(actual)})
			return
		}
		if len(exp) != len(act) {
			*out = append(*out, Mismatch{path, fmt.Sprintf("array of %d", len(exp)), fmt.Sprintf("array of %d", len(act))})
			return
		}
		for i := range exp {
			diffValue(fmt.Sprintf("%s[%d]", path, i), exp[i], act[i], out)
		}
	default:
		// numbers, bools, nil: no dynamic shape applies, compare exactly.
		if !jsonEqual(expected, actual) {
			*out = append(*out, Mismatch{path, jsonify(expected), jsonify(actual)})
		}
	}
}

func diffObject(path string, expected, actual map[string]any, out *[]Mismatch) {
	keys := make(map[string]bool, len(expected)+len(actual))
	for k := range expected {
		keys[k] = true
	}
	for k := range actual {
		keys[k] = true
	}
	sorted := make([]string, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		childPath := path + "." + k
		expVal, expOK := expected[k]
		actVal, actOK := actual[k]
		switch {
		case !expOK:
			*out = append(*out, Mismatch{childPath, "<absent>", jsonify(actVal)})
		case !actOK:
			*out = append(*out, Mismatch{childPath, jsonify(expVal), "<absent>"})
		default:
			diffValue(childPath, expVal, actVal, out)
		}
	}
}

func jsonEqual(a, b any) bool {
	ab, aerr := json.Marshal(a)
	bb, berr := json.Marshal(b)
	return aerr == nil && berr == nil && string(ab) == string(bb)
}

func jsonify(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

// FormatMismatches renders a slice of Mismatch as a multi-line, human-
// readable diff suitable for a terminal report.
func FormatMismatches(mismatches []Mismatch) string {
	lines := make([]string, len(mismatches))
	for i, m := range mismatches {
		lines[i] = m.String()
	}
	return strings.Join(lines, "\n")
}

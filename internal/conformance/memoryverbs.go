package conformance

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// MemoryVerbCase is one entry of testdata/conformance/memory_verbs/
// cases.json -- gbrain's own upstream MEMORY_VERBS v1 conformance case
// format (a bare JSON array, not a Transcript: see fixtures_test.go's own
// transcriptProtocols comment). The shape mirrors
// internal/server/memory/pinned_cases_test.go's private pinnedCase
// exactly -- that file is this format's existing reference
// implementation (T4.20) -- exported here so this package's replay
// command can drive the same cases over the wire instead of only over an
// in-process stdio pipe.
type MemoryVerbCase struct {
	Name                   string           `json:"name"`
	Verb                   string           `json:"verb"`
	Params                 map[string]any   `json:"params"`
	ExpectErrorCode        string           `json:"expectErrorCode"`
	ExpectSuggestion       bool             `json:"expectSuggestion"`
	ValidateSchema         bool             `json:"validateSchema"`
	RequiresSeededEntity   bool             `json:"requiresSeededEntity"`
	RequiresSynthesizeFlag bool             `json:"requiresSynthesizeFlag"`
	Expect                 []map[string]any `json:"expect"`
	SaveAs                 struct {
		Key  string `json:"key"`
		Path string `json:"path"`
	} `json:"saveAs"`
}

// LoadMemoryVerbCases reads and parses one memory_verbs cases.json file.
func LoadMemoryVerbCases(path string) ([]MemoryVerbCase, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("conformance: read memory verb cases %s: %w", path, err)
	}
	var cases []MemoryVerbCase
	if err := json.Unmarshal(b, &cases); err != nil {
		return nil, fmt.Errorf("conformance: parse memory verb cases %s: %w", path, err)
	}
	return cases, nil
}

// InterpolateParams resolves a case's {{marker}} and {{id:key}} template
// placeholders against a run's own marker and the ids saved by earlier
// cases' SaveAs (the identical two-placeholder vocabulary
// pinned_cases_test.go's runPinnedCases already established), returning
// the concrete params to send. An unresolved "{{" left in the encoded
// params after substitution is a fixture bug (a saveAs the case ordering
// never produced) and is reported as an error rather than sent to a live
// server as a literal template string.
func InterpolateParams(params map[string]any, marker string, saved map[string]string) (map[string]any, error) {
	encoded, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("conformance: marshal params: %w", err)
	}
	text := strings.ReplaceAll(string(encoded), "{{marker}}", marker)
	for key, id := range saved {
		text = strings.ReplaceAll(text, "{{id:"+key+"}}", id)
	}
	if strings.Contains(text, "{{") {
		return nil, fmt.Errorf("conformance: unresolved fixture interpolation %s", text)
	}
	var resolved map[string]any
	if err := json.Unmarshal([]byte(text), &resolved); err != nil {
		return nil, fmt.Errorf("conformance: unmarshal interpolated params: %w", err)
	}
	return resolved, nil
}

// LookupPath walks a dotted path (object keys or array indices, e.g.
// "facts.0.fact_id") through a decoded JSON value, mirroring
// pinned_cases_test.go's own pinnedPath.
func LookupPath(value any, path string) (any, bool) {
	for _, part := range strings.Split(path, ".") {
		switch current := value.(type) {
		case map[string]any:
			v, ok := current[part]
			if !ok {
				return nil, false
			}
			value = v
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(current) {
				return nil, false
			}
			value = current[i]
		default:
			return nil, false
		}
	}
	return value, true
}

// AssertExpectation checks one of a case's Expect entries against a
// decoded response body, mirroring pinned_cases_test.go's own
// assertPinnedExpectation but returning an error instead of calling
// t.Fatal, so a non-test caller (the CLI replay command) can report a
// mismatch as a failed case rather than aborting a process.
func AssertExpectation(body map[string]any, expect map[string]any) error {
	path, ok := expect["path"].(string)
	if !ok {
		return fmt.Errorf("expectation has no path: %v", expect)
	}
	value, exists := LookupPath(body, path)
	if needle, ok := expect["absentOrNotContains"].(string); ok {
		if !exists {
			return nil
		}
		b, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("%s: marshal for absentOrNotContains check: %w", path, err)
		}
		if strings.Contains(string(b), needle) {
			return fmt.Errorf("%s leaks %q", path, needle)
		}
		return nil
	}
	if !exists {
		return fmt.Errorf("required expectation path %s absent", path)
	}
	if expected, ok := expect["equals"]; ok && !reflect.DeepEqual(value, expected) {
		return fmt.Errorf("%s=%v want %v", path, value, expected)
	}
	if options, ok := expect["oneOf"].([]any); ok {
		found := false
		for _, option := range options {
			found = found || reflect.DeepEqual(value, option)
		}
		if !found {
			return fmt.Errorf("%s=%v outside %v", path, value, options)
		}
	}
	if typ, ok := expect["type"].(string); ok {
		valid := false
		switch typ {
		case "string":
			_, valid = value.(string)
		case "array":
			_, valid = value.([]any)
		case "number":
			_, valid = value.(float64)
		case "boolean":
			_, valid = value.(bool)
		case "object":
			_, valid = value.(map[string]any)
		default:
			return fmt.Errorf("unknown fixture type %s", typ)
		}
		if !valid {
			return fmt.Errorf("%s has type %T, want %s", path, value, typ)
		}
	}
	for _, bound := range []string{"gte", "lte"} {
		expected, ok := expect[bound].(float64)
		if !ok {
			continue
		}
		n, valid := value.(float64)
		if !valid || (bound == "gte" && n < expected) || (bound == "lte" && n > expected) {
			return fmt.Errorf("%s=%v violates %s %v", path, value, bound, expected)
		}
	}
	return nil
}

package memory

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/sirerun/serenity/internal/domain"
	"github.com/sirerun/serenity/internal/server/mcp"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

// These are the unmodified dndungu/gbrain@d35c9c9e441e fixtures. Hashes keep
// an accidental fixture edit from redefining the expected protocol behavior.
const pinnedCasesSHA = "44bc81bd1bdec006c69d5c81f2513b0410c63da8e0d98138f48068eb1e9c0edf"
const pinnedSchemasSHA = "969266be1be1edbd2088ba26176697df22b1248d1efb5872988fb5bf6f341a9d"

type pinnedCase struct {
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
type pinnedCapture struct {
	verb    string
	value   map[string]any
	isError bool
}
type pinnedSchemas struct {
	responses map[string]*jsonschema.Schema
	failure   *jsonschema.Schema
}
type pinnedOfflineLoader struct{}

func (pinnedOfflineLoader) Load(url string) (any, error) {
	return nil, fmt.Errorf("pinned schemas cannot load external resource %s", url)
}

func pinnedFixture(t *testing.T, name, wantHash string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	if hex.EncodeToString(sum[:]) != wantHash {
		t.Fatalf("pinned fixture %s changed", name)
	}
	return b
}
func compilePinnedSchemas(t *testing.T) pinnedSchemas {
	t.Helper()
	var registry struct {
		Responses map[string]json.RawMessage `json:"responses"`
		Error     json.RawMessage            `json:"error"`
	}
	if err := json.Unmarshal(pinnedFixture(t, "pinned-response-schemas.json", pinnedSchemasSHA), &registry); err != nil {
		t.Fatal(err)
	}
	compile := func(name string, raw json.RawMessage) *jsonschema.Schema {
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			t.Fatal(err)
		}
		c := jsonschema.NewCompiler()
		c.DefaultDraft(jsonschema.Draft2020)
		c.UseLoader(pinnedOfflineLoader{})
		id := "urn:serenity:test:pinned:" + name
		if err := c.AddResource(id, doc); err != nil {
			t.Fatal(err)
		}
		schema, err := c.Compile(id)
		if err != nil {
			t.Fatal(err)
		}
		return schema
	}
	result := pinnedSchemas{responses: map[string]*jsonschema.Schema{}}
	if len(registry.Responses) != 5 {
		t.Fatalf("pinned response inventory has %d verbs", len(registry.Responses))
	}
	for _, verb := range []string{"recall", "remember", "entity", "synthesize", "forget"} {
		result.responses[verb] = compile(verb, registry.Responses[verb])
	}
	result.failure = compile("error", registry.Error)
	return result
}

// The pinned error JSON Schema only requires error/message, while the adopted
// prose contract requires version and corrective suggestion on every error.
// Enforce both sources independently without weakening the vendored schema.
func (s pinnedSchemas) check(c pinnedCapture) error {
	schema := s.responses[c.verb]
	if c.isError {
		schema = s.failure
	}
	if schema == nil {
		return fmt.Errorf("missing pinned schema for %s", c.verb)
	}
	if err := schema.Validate(c.value); err != nil {
		return err
	}
	if c.value["protocol_version"] != float64(1) {
		return fmt.Errorf("missing integer protocol_version=1")
	}
	_, hasError := c.value["error"]
	if hasError != c.isError {
		return fmt.Errorf("MCP isError disagrees with domain error body")
	}
	if c.isError {
		suggestion, ok := c.value["suggestion"].(string)
		if !ok || strings.TrimSpace(suggestion) == "" {
			return fmt.Errorf("domain error lacks corrective suggestion")
		}
		if c.value["error"] == "budget_unsatisfiable" {
			return fmt.Errorf("reserved error code emitted")
		}
	}
	return nil
}
func (s pinnedSchemas) validate(t *testing.T, c pinnedCapture) {
	t.Helper()
	if err := s.check(c); err != nil {
		t.Fatalf("live %s response violates pinned contract: %v; response=%v", c.verb, err, c.value)
	}
}

// No case is skipped: seeded-entity and synthesis cases run with real local
// state and a deliberately unavailable composer. Full CI transport conformance
// remains separate from this package's real stdio protocol regression.
func TestMemoryV1AllPinnedCases(t *testing.T) { runPinnedCases(t, compilePinnedSchemas(t)) }

func TestMemoryV1PinnedWireContracts(t *testing.T) {
	schemas := compilePinnedSchemas(t)
	captures := runPinnedCases(t, schemas)
	seen := map[string]bool{}
	for i, capture := range captures {
		seen[capture.verb] = true
		schemas.validate(t, capture)
		// Mutate captured emitted bytes, not a hand-written substitute response.
		encoded, err := json.Marshal(capture.value)
		if err != nil {
			t.Fatal(err)
		}
		var mutated map[string]any
		if err := json.Unmarshal(encoded, &mutated); err != nil {
			t.Fatal(err)
		}
		mutated["protocol_version"] = "1"
		schema := schemas.responses[capture.verb]
		if capture.isError {
			schema = schemas.failure
		}
		if err := schema.Validate(mutated); err == nil {
			t.Fatalf("capture %d %s accepted string protocol version", i, capture.verb)
		}
		if capture.isError {
			mutated["protocol_version"] = float64(1)
			mutated["error"] = "invented_error_code"
			if err := schema.Validate(mutated); err == nil {
				t.Fatal("pinned error schema accepted unknown error code")
			}
		}
	}
	if len(seen) != 5 {
		t.Fatalf("live schema coverage contains %d verbs, want 5", len(seen))
	}
}

func runPinnedCases(t *testing.T, schemas pinnedSchemas) []pinnedCapture {
	t.Helper()
	var cases []pinnedCase
	if err := json.Unmarshal(pinnedFixture(t, "memory-cases-upstream.json", pinnedCasesSHA), &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 17 {
		t.Fatalf("selected %d pinned cases, want exactly 17", len(cases))
	}
	const marker = "serenity-pinned-v1"
	root := gitRepoFixture(t)
	deps, closeDeps := testDeps(t, root)
	t.Cleanup(closeDeps)
	if deps.Composer != nil {
		t.Fatal("pinned unavailable fixture unexpectedly has a composer")
	}
	page := &store.EntityPage{Entity: domain.Entity{Type: "people", Slug: "conformance-" + marker}}
	if _, _, err := writer.Fence(deps.Queue, deps.Fence, page); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Flush(deps.Queue, root); err != nil {
		t.Fatal(err)
	}
	session := newPinnedSession(t, New(deps).Tools())
	listed := session.request(t, "tools/list", map[string]any{})
	var catalog struct {
		Tools []struct {
			Name        string         `json:"name"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(listed, &catalog); err != nil {
		t.Fatal(err)
	}
	names := []string{}
	required := map[string][]string{"recall": {}, "remember": {"fact", "provenance"}, "entity": {"name"}, "synthesize": {"question"}, "forget": {"id"}}
	for _, tool := range catalog.Tools {
		names = append(names, tool.Name)
		want, ok := required[tool.Name]
		if !ok {
			t.Fatalf("unexpected live tool %s", tool.Name)
		}
		got := []string{}
		if values, ok := tool.InputSchema["required"].([]any); ok {
			for _, v := range values {
				str, ok := v.(string)
				if !ok {
					t.Fatal("nonstring required field")
				}
				got = append(got, str)
			}
		}
		sort.Strings(got)
		sort.Strings(want)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s required fields=%v, pinned=%v", tool.Name, got, want)
		}
	}
	sort.Strings(names)
	if !reflect.DeepEqual(names, []string{"entity", "forget", "recall", "remember", "synthesize"}) {
		t.Fatalf("actual registry=%v", names)
	}
	saved := map[string]string{}
	captures := []pinnedCapture{}
	executed := 0
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			encoded, err := json.Marshal(tc.Params)
			if err != nil {
				t.Fatal(err)
			}
			text := strings.ReplaceAll(string(encoded), "{{marker}}", marker)
			for key, id := range saved {
				text = strings.ReplaceAll(text, "{{id:"+key+"}}", id)
			}
			if strings.Contains(text, "{{") {
				t.Fatalf("unresolved fixture interpolation %s", text)
			}
			var params map[string]any
			if err := json.Unmarshal([]byte(text), &params); err != nil {
				t.Fatal(err)
			}
			executed++
			raw := session.request(t, "tools/call", map[string]any{"name": tc.Verb, "arguments": params})
			var result mcp.Result
			if err := json.Unmarshal(raw, &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Content) != 1 || result.Content[0].Type != "text" {
				t.Fatalf("unexpected tools/call content: %s", raw)
			}
			var value map[string]any
			decoder := json.NewDecoder(strings.NewReader(result.Content[0].Text))
			if err := decoder.Decode(&value); err != nil {
				t.Fatalf("domain body is not JSON: %v", err)
			}
			var extra any
			if err := decoder.Decode(&extra); err != io.EOF {
				t.Fatal("domain body has trailing content")
			}
			capture := pinnedCapture{verb: tc.Verb, value: value, isError: result.IsError}
			captures = append(captures, capture)
			schemas.validate(t, capture)
			wantError := tc.ExpectErrorCode
			if tc.RequiresSynthesizeFlag {
				wantError = "unavailable"
			}
			if wantError != "" {
				if !result.IsError || value["error"] != wantError {
					t.Fatalf("error=%v isError=%t; want %s", value, result.IsError, wantError)
				}
				// The adopted error envelope requires populated suggestions even where the
				// older fixture marks the suggestion assertion optional.
				suggestion, ok := value["suggestion"].(string)
				if !ok || strings.TrimSpace(suggestion) == "" {
					t.Fatal("missing corrective suggestion")
				}
			} else if result.IsError {
				t.Fatalf("unexpected tool error: %v", value)
			}
			for _, expect := range tc.Expect {
				assertPinnedExpectation(t, value, expect)
			}
			if tc.SaveAs.Key != "" {
				v, ok := pinnedPath(value, tc.SaveAs.Path)
				if !ok {
					t.Fatal("missing saved response path")
				}
				id, ok := v.(string)
				if !ok || id == "" {
					t.Fatal("saved ID is not a nonempty string")
				}
				saved[tc.SaveAs.Key] = id
			}
		})
	}
	if executed != 17 {
		t.Errorf("EXECUTED_PINNED_CASES=%d; want 17", executed)
	}
	t.Logf("EXECUTED_PINNED_CASES=%d SEEDED_ENTITY=true SYNTHESIS_UNAVAILABLE_EXECUTED=true", executed)
	return captures
}

func pinnedPath(value any, path string) (any, bool) {
	for _, part := range strings.Split(path, ".") {
		switch current := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = current[part]
			if !ok {
				return nil, false
			}
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
func assertPinnedExpectation(t *testing.T, body map[string]any, expect map[string]any) {
	t.Helper()
	path, ok := expect["path"].(string)
	if !ok {
		t.Fatal("fixture expectation has no path")
	}
	value, exists := pinnedPath(body, path)
	if needle, ok := expect["absentOrNotContains"].(string); ok {
		if !exists {
			return
		}
		b, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), needle) {
			t.Fatalf("%s leaks %q", path, needle)
		}
		return
	}
	if !exists {
		t.Fatalf("required expectation path %s absent", path)
	}
	if expected, ok := expect["equals"]; ok && !reflect.DeepEqual(value, expected) {
		t.Fatalf("%s=%v want %v", path, value, expected)
	}
	if options, ok := expect["oneOf"].([]any); ok {
		found := false
		for _, option := range options {
			found = found || reflect.DeepEqual(value, option)
		}
		if !found {
			t.Fatalf("%s=%v outside %v", path, value, options)
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
			t.Fatalf("unknown fixture type %s", typ)
		}
		if !valid {
			t.Fatalf("%s has type %T, want %s", path, value, typ)
		}
	}
	for _, bound := range []string{"gte", "lte"} {
		if expected, ok := expect[bound].(float64); ok {
			n, valid := value.(float64)
			if !valid || (bound == "gte" && n < expected) || (bound == "lte" && n > expected) {
				t.Fatalf("%s=%v violates %s %v", path, value, bound, expected)
			}
		}
	}
}

type pinnedSession struct {
	input  *io.PipeWriter
	frames chan []byte
	nextID int
}

func newPinnedSession(t *testing.T, tools []mcp.Tool) *pinnedSession {
	t.Helper()
	server, err := mcp.New("pinned-test", tools)
	if err != nil {
		t.Fatal(err)
	}
	input, in := io.Pipe()
	out, output := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	s := &pinnedSession{input: in, frames: make(chan []byte, 64)}
	done, scanned := make(chan error, 1), make(chan error, 1)
	go func() { done <- server.Serve(ctx, input, output); _ = output.Close() }()
	go func() {
		defer close(s.frames)
		scanner := bufio.NewScanner(out)
		scanner.Buffer(make([]byte, 4096), mcp.MaxFrameBytes)
		for scanner.Scan() {
			s.frames <- bytes.Clone(scanner.Bytes())
		}
		scanned <- scanner.Err()
		_ = out.Close()
	}()
	t.Cleanup(func() {
		cancel()
		_ = in.Close()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("MCP session did not stop")
		}
		select {
		case err := <-scanned:
			if err != nil && !errors.Is(err, io.ErrClosedPipe) {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("MCP output reader did not stop")
		}
	})
	result := s.request(t, "initialize", map[string]any{"protocolVersion": mcp.ProtocolVersion, "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "pinned-tests", "version": "1"}})
	var init map[string]any
	if err := json.Unmarshal(result, &init); err != nil || init["protocolVersion"] != mcp.ProtocolVersion {
		t.Fatalf("invalid initialize response %s", result)
	}
	s.send(t, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	return s
}
func (s *pinnedSession) send(t *testing.T, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.input.Write(append(raw, '\n')); err != nil {
		t.Fatal(err)
	}
}
func (s *pinnedSession) request(t *testing.T, method string, params any) json.RawMessage {
	t.Helper()
	s.nextID++
	s.send(t, map[string]any{"jsonrpc": "2.0", "id": s.nextID, "method": method, "params": params})
	select {
	case raw, ok := <-s.frames:
		if !ok {
			t.Fatal("MCP output closed before reply")
		}
		var frame map[string]json.RawMessage
		if err := json.Unmarshal(raw, &frame); err != nil {
			t.Fatal(err)
		}
		if string(frame["jsonrpc"]) != `"2.0"` || string(frame["id"]) != strconv.Itoa(s.nextID) {
			t.Fatalf("unexpected JSON-RPC frame %s", raw)
		}
		if frame["protocol_version"] != nil {
			t.Fatal("domain version leaked into JSON-RPC frame")
		}
		if frame["error"] != nil {
			t.Fatalf("unexpected JSON-RPC error: %s", frame["error"])
		}
		return frame["result"]
	case <-time.After(15 * time.Second):
		t.Fatal("MCP reply timed out")
		return nil
	}
}

// Each mutation starts from a separately validated real tools/call capture and
// must change it before the independent validator can count a rejection.
func TestMemoryV1IndependentNegativeMutations(t *testing.T) {
	schemas := compilePinnedSchemas(t)
	captures := runPinnedCases(t, schemas)
	selectCapture := func(verb string, isError bool, field string) pinnedCapture {
		t.Helper()
		for _, c := range captures {
			if c.verb != verb || c.isError != isError {
				continue
			}
			if field != "" {
				value, ok := c.value[field]
				if !ok {
					continue
				}
				if values, ok := value.([]any); ok && len(values) == 0 {
					continue
				}
				if value == nil {
					continue
				}
			}
			schemas.validate(t, c)
			return c
		}
		t.Fatalf("no actual %s error=%t capture with %s to mutate", verb, isError, field)
		return pinnedCapture{}
	}
	failure := selectCapture("remember", true, "")
	recall := selectCapture("recall", false, "facts")
	search := selectCapture("recall", false, "results")
	remember := selectCapture("remember", false, "id")
	entity := selectCapture("entity", false, "card")
	forget := selectCapture("forget", false, "expired")
	mutations := []struct {
		name     string
		original pinnedCapture
		mutate   func(map[string]any)
	}{
		{"nested-error", failure, func(v map[string]any) { v["error"] = map[string]any{"code": v["error"], "message": v["message"]} }},
		{"unknown-error-enum", failure, func(v map[string]any) { v["error"] = "invented" }},
		{"empty-suggestion", failure, func(v map[string]any) { v["suggestion"] = " " }},
		{"missing-error-version", failure, func(v map[string]any) { delete(v, "protocol_version") }},
		{"missing-facts", recall, func(v map[string]any) { delete(v, "facts") }},
		{"numeric-fact-id", recall, func(v map[string]any) { v["facts"].([]any)[0].(map[string]any)["fact_id"] = float64(42) }},
		{"unknown-fact-kind", recall, func(v map[string]any) { v["facts"].([]any)[0].(map[string]any)["kind"] = "invented" }},
		{"unknown-visibility", recall, func(v map[string]any) { v["facts"].([]any)[0].(map[string]any)["visibility"] = "invented" }},
		{"unknown-search-evidence", search, func(v map[string]any) { v["results"].([]any)[0].(map[string]any)["evidence"] = "invented" }},
		{"unknown-create-safety", search, func(v map[string]any) { v["results"].([]any)[0].(map[string]any)["create_safety"] = "invented" }},
		{"numeric-remember-id", remember, func(v map[string]any) { v["id"] = float64(42) }},
		{"unknown-remember-status", remember, func(v map[string]any) { v["status"] = "invented" }},
		{"malformed-entity-card", entity, func(v map[string]any) { v["card"] = "not-an-object" }},
		{"string-forget-expired", forget, func(v map[string]any) { v["expired"] = "true" }},
	}
	rejected := 0
	for _, mutation := range mutations {
		t.Run("reject-"+mutation.name, func(t *testing.T) {
			schemas.validate(t, mutation.original)
			original, err := json.Marshal(mutation.original.value)
			if err != nil {
				t.Fatal(err)
			}
			var value map[string]any
			if err := json.Unmarshal(original, &value); err != nil {
				t.Fatal(err)
			}
			mutation.mutate(value)
			changed, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Equal(original, changed) {
				t.Fatal("mutation did not change the real capture")
			}
			altered := pinnedCapture{verb: mutation.original.verb, value: value, isError: mutation.original.isError}
			if err := schemas.check(altered); err == nil {
				t.Fatal("independent validator accepted semantic mutation")
			}
			rejected++
		})
	}
	if rejected != 14 {
		t.Errorf("REJECTED_SEMANTIC_MUTATIONS=%d; want14", rejected)
	}
	t.Logf("REJECTED_SEMANTIC_MUTATIONS=%d ORIGINALS_VALIDATED=%d", rejected, len(mutations))
}

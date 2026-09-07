package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirerun/serenity/internal/server/mcp"
)

func TestMemoryV1ErrorsAcrossMCP(t *testing.T) {
	h, _ := newTestHandlers(t)
	session := newPinnedSession(t, h.Tools())
	schemas := compilePinnedSchemas(t)
	cases := []struct {
		name, verb, code string
		args             map[string]any
	}{
		{"missing provenance", "remember", "invalid_params", map[string]any{"fact": "fixture"}},
		{"wrong provenance type", "remember", "invalid_params", map[string]any{"fact": "fixture", "provenance": 12}},
		{"null provenance", "remember", "invalid_params", map[string]any{"fact": "fixture", "provenance": nil}},
		{"blank provenance", "remember", "provenance_required", map[string]any{"fact": "fixture", "provenance": " \n "}},
		{"long attribution", "remember", "invalid_params", map[string]any{"fact": "fixture", "provenance": strings.Repeat("p", 501)}},
		{"blank fact", "remember", "invalid_params", map[string]any{"fact": " ", "provenance": "fixture"}},
		{"invalid TTL", "remember", "invalid_params", map[string]any{"fact": "fixture", "provenance": "fixture", "ttl": "P30D"}},
		{"wrong kind", "remember", "invalid_params", map[string]any{"fact": "fixture", "provenance": "fixture", "kind": "accepted_claim"}},
		{"wrong visibility", "remember", "invalid_params", map[string]any{"fact": "fixture", "provenance": "fixture", "visibility": "public"}},
		{"fractional budget", "recall", "invalid_params", map[string]any{"budget_tokens": 1.5}},
		{"negative limit", "recall", "invalid_params", map[string]any{"limit": -1}},
		{"invalid date", "synthesize", "invalid_params", map[string]any{"question": "fixture", "since": "last year"}},
		{"reversed dates", "synthesize", "invalid_params", map[string]any{"question": "fixture", "since": "2030-01-01", "until": "2020-01-01"}},
		{"unknown ID", "forget", "not_found", map[string]any{"id": strings.Repeat("a", 64)}},
		{"wrong ID type", "forget", "invalid_params", map[string]any{"id": 42}},
		{"missing entity name", "entity", "invalid_params", map[string]any{"slug": "old-guessed-contract"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := session.request(t, "tools/call", map[string]any{"name": tc.verb, "arguments": tc.args})
			var result mcp.Result
			if err := json.Unmarshal(raw, &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Content) != 1 {
				t.Fatalf("content=%+v", result)
			}
			var body map[string]any
			if err := json.Unmarshal([]byte(result.Content[0].Text), &body); err != nil {
				t.Fatalf("non-JSON error: %q", result.Content[0].Text)
			}
			reviewMemoryError(t, body, result.IsError, tc.code)
			schemas.validate(t, pinnedCapture{verb: tc.verb, value: body, isError: result.IsError})
		})
	}
	t.Run("writer failure", func(t *testing.T) {
		h.deps.Queue.Close()
		raw := session.request(t, "tools/call", map[string]any{"name": "remember", "arguments": map[string]any{"fact": "closed queue", "provenance": "fixture"}})
		var result mcp.Result
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(result.Content[0].Text), &body); err != nil {
			t.Fatal(err)
		}
		reviewMemoryError(t, body, result.IsError, "internal")
		schemas.validate(t, pinnedCapture{verb: "remember", value: body, isError: true})
		if strings.Contains(result.Content[0].Text, h.deps.Root) {
			t.Fatal("internal path leaked")
		}
	})
}

func TestMemoryV1CorruptSourceFailsClosedAcrossTools(t *testing.T) {
	h, root := newTestHandlers(t)
	entityReviewPage(t, h, "person", "alice", "Alice")
	saved := acceptanceCall(t, h, "remember", map[string]any{"fact": "corruption marker", "provenance": "fixture", "entity": "person/alice"})
	id := saved["id"].(string)
	path := filepath.Join(h.deps.Sources.DirFor(id), "bytes")
	damaged := []byte("corrupted canonical source")
	if err := os.WriteFile(path, damaged, 0600); err != nil {
		t.Fatal(err)
	}
	provider := &wireReviewCompleter{text: "must not be called"}
	h.deps.Composer = provider
	session := newPinnedSession(t, h.Tools())
	for _, tc := range []struct {
		verb string
		args map[string]any
	}{
		{"recall", map[string]any{"query": "corruption"}},
		{"remember", map[string]any{"fact": "replacement", "provenance": "fixture"}},
		{"entity", map[string]any{"name": "person/alice"}},
		{"synthesize", map[string]any{"question": "corruption"}},
		{"forget", map[string]any{"id": id}},
	} {
		t.Run(tc.verb, func(t *testing.T) {
			raw := session.request(t, "tools/call", map[string]any{"name": tc.verb, "arguments": tc.args})
			var result mcp.Result
			if err := json.Unmarshal(raw, &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Content) != 1 {
				t.Fatalf("result=%v", result)
			}
			var body map[string]any
			if err := json.Unmarshal([]byte(result.Content[0].Text), &body); err != nil {
				t.Fatal(err)
			}
			reviewMemoryError(t, body, result.IsError, "internal")
			if strings.Contains(result.Content[0].Text, root) {
				t.Fatal("internal source path exposed")
			}
		})
	}
	current, err := os.ReadFile(path)
	if err != nil || string(current) != string(damaged) {
		t.Fatalf("corrupt authority silently overwritten: %s %v", current, err)
	}
	if provider.prompt != "" {
		t.Fatal("corrupt source reached provider")
	}
}

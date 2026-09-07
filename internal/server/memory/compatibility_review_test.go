package memory

import (
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/server/mcp"
)

// These independent expectations come from MEMORY_VERBS_v1.md and the response
// registry at dndungu/gbrain d35c9c9e441e, not Serenity's pre-repair wire types.
// Exercise real initialize/initialized/tools/call framing: schema rejection is
// part of the observed contract, not something the test bypasses.
func reviewMemoryCall(t *testing.T, name string, args map[string]any) (map[string]any, bool) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("real fixture requires git: %v", err)
	}
	h, _ := newTestHandlers(t)
	t.Cleanup(h.deps.Queue.Close)
	srv, err := mcp.New("compatibility-review", h.Tools())
	if err != nil {
		t.Fatal(err)
	}
	input, send := io.Pipe()
	receive, output := io.Pipe()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ctx, input, output) }()
	t.Cleanup(func() {
		cancel()
		_ = send.Close()
		_ = receive.Close()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("MCP shutdown: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Error("MCP server did not join its workers")
		}
	})
	enc, dec := json.NewEncoder(send), json.NewDecoder(receive)
	sendFrame := func(v any) {
		t.Helper()
		if err := enc.Encode(v); err != nil {
			t.Fatalf("write MCP frame: %v", err)
		}
	}
	readFrame := func() map[string]json.RawMessage {
		t.Helper()
		var frame map[string]json.RawMessage
		if err := dec.Decode(&frame); err != nil {
			t.Fatalf("read MCP frame: %v", err)
		}
		if frame["error"] != nil {
			t.Fatalf("unexpected JSON-RPC error: %s", frame["error"])
		}
		return frame
	}
	sendFrame(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{
		"protocolVersion": mcp.ProtocolVersion, "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "independent-review", "version": "1"},
	}})
	readFrame()
	sendFrame(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	sendFrame(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
	frame := readFrame()
	var result mcp.Result
	if err := json.Unmarshal(frame["result"], &result); err != nil {
		t.Fatalf("decode tool result: %v", err)
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatalf("expected one JSON text envelope, got %+v", result)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].Text), &body); err != nil {
		t.Fatalf("%s must return a JSON memory envelope even on failure; got %q", name, result.Content[0].Text)
	}
	if body["protocol_version"] != float64(1) {
		t.Errorf("protocol_version = %#v, want integer 1", body["protocol_version"])
	}
	return body, result.IsError
}

func reviewMemoryError(t *testing.T, body map[string]any, isError bool, code string) {
	t.Helper()
	if !isError {
		t.Error("MCP isError must be true for a memory error")
	}
	if body["error"] != code {
		t.Errorf("top-level error = %#v, want string %q", body["error"], code)
	}
	for _, field := range []string{"message", "suggestion"} {
		text, ok := body[field].(string)
		if !ok || strings.TrimSpace(text) == "" {
			t.Errorf("%s = %#v, want populated top-level string", field, body[field])
		}
	}
}

func TestMemoryV1IndependentPinnedContract(t *testing.T) {
	cases := []struct {
		name string
		args map[string]any
	}{
		{"recall", map[string]any{"query": "independentreviewabsent"}},
		{"remember", map[string]any{"fact": "Independent review chose a local fixture.", "provenance": "independent protocol review"}},
		{"entity", map[string]any{"name": "independent-review-absent"}},
		{"synthesize", map[string]any{"question": "What did the independent review decide?"}},
		{"forget", map[string]any{"id": "independent-review-unknown-id"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, failed := reviewMemoryCall(t, tc.name, tc.args)
			switch tc.name {
			case "synthesize":
				reviewMemoryError(t, body, failed, "unavailable")
			case "forget":
				reviewMemoryError(t, body, failed, "not_found")
			case "recall":
				if failed {
					t.Fatalf("valid recall failed: %#v", body)
				}
				if facts, ok := body["facts"].([]any); !ok || len(facts) != 0 {
					t.Errorf("empty brain must return facts:[], got %#v", body["facts"])
				}
				if body["total"] != float64(0) {
					t.Errorf("total = %#v, want 0", body["total"])
				}
				if _, ok := body["results"].([]any); !ok {
					t.Errorf("query arm must return results array, got %#v", body["results"])
				}
				for _, key := range []string{"budget", "budget_tokens", "budget_used", "dropped_count"} {
					if _, present := body[key]; present {
						t.Errorf("omitted budget_tokens must not emit budget metadata %q", key)
					}
				}
			case "remember":
				if failed || body["status"] != "inserted" {
					t.Fatalf("fresh attributed fact must insert: isError=%v body=%#v", failed, body)
				}
				if id, ok := body["id"].(string); !ok || id == "" {
					t.Errorf("remember id must be nonempty opaque string, got %#v", body["id"])
				}
				for _, key := range []string{"entity_slug", "valid_until"} {
					if value, present := body[key]; !present || value != nil {
						t.Errorf("omitted %s must echo explicit null, got %#v (present=%v)", key, value, present)
					}
				}
			case "entity":
				if failed || body["found"] != false {
					t.Errorf("entity miss must succeed with found:false: %#v", body)
				}
				if _, ok := body["latency_ms"].(float64); !ok {
					t.Errorf("entity miss must report numeric latency_ms, got %#v", body["latency_ms"])
				}
				if _, ok := body["suggestions"].([]any); !ok {
					t.Errorf("entity miss must return suggestions array, got %#v", body["suggestions"])
				}
			}
		})
	}
	t.Run("errors", reviewMemoryErrorEnvelopes)
}

func reviewMemoryErrorEnvelopes(t *testing.T) {
	cases := []struct {
		name, verb, code string
		args             map[string]any
	}{
		{"missing_provenance", "remember", "invalid_params", map[string]any{"fact": "Review attribution is mandatory."}},
		{"blank_provenance", "remember", "provenance_required", map[string]any{"fact": "Review attribution is mandatory.", "provenance": "   "}},
		{"invalid_ttl", "remember", "invalid_params", map[string]any{"fact": "Review TTL syntax.", "provenance": "review", "ttl": "P30D"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, failed := reviewMemoryCall(t, tc.verb, tc.args)
			reviewMemoryError(t, body, failed, tc.code)
		})
	}
}

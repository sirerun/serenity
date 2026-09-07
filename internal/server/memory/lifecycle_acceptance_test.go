package memory

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/sirerun/serenity/internal/index"
	"github.com/sirerun/serenity/internal/store"
	"github.com/sirerun/serenity/internal/writer"
)

func acceptanceCall(t *testing.T, h *Handlers, verb string, args any) map[string]any {
	t.Helper()
	for _, tool := range h.Tools() {
		if tool.Name != verb {
			continue
		}
		result, err := tool.Handler(t.Context(), mustMarshal(t, args))
		if err != nil {
			t.Fatalf("%s public Go error: %v", verb, err)
		}
		if len(result.Content) != 1 {
			t.Fatalf("%s content: %+v", verb, result)
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(result.Content[0].Text), &body); err != nil {
			t.Fatal(err)
		}
		if body["protocol_version"] != float64(1) {
			t.Fatalf("%s version=%v", verb, body)
		}
		_, domainError := body["error"]
		if result.IsError != domainError {
			t.Fatalf("%s mismatched error flags: %+v", verb, result)
		}
		return body
	}
	t.Fatalf("missing tool %s", verb)
	return nil
}

func TestMemoryV1DurableRoundTrip(t *testing.T) {
	h, root := newTestHandlers(t)
	fact, attribution := "  Verbatim durable fact\n", "  Heard directly from the user\n"
	request := map[string]any{"fact": fact, "provenance": attribution, "entity": "people/alice", "kind": "commitment", "ttl": "2030-01-01T00:00:00.123456789Z"}
	saved := acceptanceCall(t, h, "remember", request)
	id, ok := saved["id"].(string)
	if !ok || len(id) != 64 || saved["status"] != "inserted" || saved["degraded_dedup"] != true {
		t.Fatalf("insert=%v", saved)
	}
	duplicate := acceptanceCall(t, h, "remember", request)
	if duplicate["id"] != id || duplicate["status"] != "duplicate" {
		t.Fatalf("duplicate=%v", duplicate)
	}
	rawBefore, _, err := h.deps.Sources.Read(id)
	if err != nil {
		t.Fatal(err)
	}
	h.deps.Queue.Close()
	if _, err := writer.Flush(h.deps.Queue, root); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "ls-files", "brain/sources")
	cmd.Dir = root
	tracked, err := cmd.Output()
	if err != nil || !strings.Contains(string(tracked), id) {
		t.Fatalf("canonical source not committed: %v %s", err, tracked)
	}
	deps, closeFn := testDeps(t, root)
	t.Cleanup(closeFn)
	reopened := New(deps)
	result := acceptanceCall(t, reopened, "recall", map[string]any{"entity": "people/alice"})
	facts := result["facts"].([]any)
	if len(facts) != 1 {
		t.Fatalf("reopened=%v", result)
	}
	got := facts[0].(map[string]any)
	if got["fact_id"] != id || got["fact"] != fact || got["provenance"] != attribution || got["kind"] != "commitment" || got["valid_until"] != "2030-01-01T00:00:00.123456789Z" {
		t.Fatalf("roundtrip=%v", got)
	}
	legacy := got["id"].(float64)
	if legacy <= 0 || legacy > 9007199254740991 {
		t.Fatalf("legacy id=%v", legacy)
	}
	first := acceptanceCall(t, reopened, "forget", map[string]any{"id": id, "reason": "  corrected by user  "})
	if first["expired"] != true || first["reason"] != "  corrected by user  " {
		t.Fatalf("forget=%v", first)
	}
	repeat := acceptanceCall(t, reopened, "forget", map[string]any{"id": id})
	if repeat["expired"] != false || repeat["reason"] != nil {
		t.Fatalf("repeat=%v", repeat)
	}
	reopened.deps.Queue.Close()
	if _, err := writer.Flush(reopened.deps.Queue, root); err != nil {
		t.Fatal(err)
	}
	thirdDeps, thirdClose := testDeps(t, root)
	t.Cleanup(thirdClose)
	third := New(thirdDeps)
	after := acceptanceCall(t, third, "recall", map[string]any{})
	if after["total"] != float64(0) {
		t.Fatalf("forgotten fact resurrected: %v", after)
	}
	rawAfter, _, err := third.deps.Sources.Read(id)
	if err != nil || string(rawAfter) != string(rawBefore) {
		t.Fatalf("audit source changed: %v", err)
	}
	fresh := acceptanceCall(t, third, "remember", request)
	if fresh["id"] == id || fresh["status"] != "inserted" {
		t.Fatalf("expired record reused: %v", fresh)
	}
}

func TestMemoryV1FactsFirstBudget(t *testing.T) {
	h, _ := newTestHandlers(t)
	fact := strings.Repeat("budgetmarker", 20)
	acceptanceCall(t, h, "remember", map[string]any{"fact": fact, "provenance": "budget fixture", "entity": "people/alice"})
	if err := h.deps.Index.InsertChunk(t.Context(), "source:budget", "", "budgetmarker second result", "", "note"); err != nil {
		t.Fatal(err)
	}
	all := acceptanceCall(t, h, "recall", map[string]any{"query": "budgetmarker"})
	if _, ok := all["budget_tokens"]; ok {
		t.Fatalf("implicit budget metadata: %v", all)
	}
	for _, budget := range []int{0, 1, len(fact) / 4, 1000} {
		t.Run(string(rune('A'+budget%26)), func(t *testing.T) {
			got := acceptanceCall(t, h, "recall", map[string]any{"query": "budgetmarker", "budget_tokens": budget})
			used := got["budget_used"].(float64)
			dropped := got["dropped_count"].(float64)
			facts, results := got["facts"].([]any), got["results"].([]any)
			if used > float64(budget) || dropped != float64(2-len(facts)-len(results)) {
				t.Fatalf("accounting: %v", got)
			}
			if budget <= 1 && (used != 0 || len(facts)+len(results) != 0) {
				t.Fatalf("word-based estimate or zero leak: %v", got)
			}
			if budget == len(fact)/4 && (len(facts) != 1 || len(results) != 0 || used != float64(len(fact)/4)) {
				t.Fatalf("facts not packed first: %v", got)
			}
			if budget == 1000 && (len(facts) != 1 || len(results) != 1) {
				t.Fatalf("large budget lost candidates: %v", got)
			}
		})
	}
	scoped := acceptanceCall(t, h, "recall", map[string]any{"query": "budgetmarker", "entity": "people/nobody", "since": "2020-01-01"})
	if scoped["total"] != float64(0) || len(scoped["results"].([]any)) != 1 {
		t.Fatalf("entity scope affected query arm: %v", scoped)
	}
	zero := acceptanceCall(t, h, "recall", map[string]any{"query": "budgetmarker", "limit": 0})
	if zero["total"] != float64(0) || len(zero["results"].([]any)) != 0 {
		t.Fatalf("zero limit: %v", zero)
	}
}

func TestMemoryV1PrivateForgetAndStaleIndex(t *testing.T) {
	h, root := newTestHandlers(t)
	secret := acceptanceCall(t, h, "remember", map[string]any{"fact": "private stale marker", "provenance": "private attribution", "visibility": "private"})
	world := acceptanceCall(t, h, "remember", map[string]any{"fact": "public stale marker", "provenance": "world attribution"})
	if err := index.Rebuild(context.Background(), root, h.deps.Config, h.deps.Index); err != nil {
		t.Fatal(err)
	}
	denied := acceptanceCall(t, h, "forget", map[string]any{"id": secret["id"]})
	if denied["error"] != "scope_denied" {
		t.Fatalf("private forget=%v", denied)
	}
	proj, err := store.LoadMemoryProjection(h.deps.Sources)
	if err != nil {
		t.Fatal(err)
	}
	rec, ok := proj.Get(secret["id"].(string))
	if !ok || rec.Expired(testNow) {
		t.Fatal("private fact mutated")
	}
	acceptanceCall(t, h, "forget", map[string]any{"id": world["id"]})
	response := acceptanceCall(t, h, "recall", map[string]any{"query": "marker"})
	if response["total"] != float64(0) || len(response["results"].([]any)) != 0 {
		t.Fatalf("private/forgotten stale index leak: %v", response)
	}
}

func TestMemoryV1TTLValidation(t *testing.T) {
	h, _ := newTestHandlers(t)
	for _, ttl := range []string{"P30D", "0m", "99999999999999999999999999999d", "9223372036854775807h"} {
		got := acceptanceCall(t, h, "remember", map[string]any{"fact": "TTL boundary", "provenance": "fixture", "ttl": ttl})
		if got["error"] != "invalid_params" {
			t.Fatalf("ttl %s: %v", ttl, got)
		}
	}
	for _, tc := range []struct {
		ttl      string
		duration time.Duration
	}{{"30d", 30 * 24 * time.Hour}, {"12h", 12 * time.Hour}, {"45m", 45 * time.Minute}} {
		got := acceptanceCall(t, h, "remember", map[string]any{"fact": "TTL boundary", "provenance": "fixture", "ttl": tc.ttl})
		if got["valid_until"] != testNow.Add(tc.duration).Format(time.RFC3339Nano) {
			t.Fatalf("ttl %s: %v", tc.ttl, got)
		}
	}
	unicode := acceptanceCall(t, h, "remember", map[string]any{"fact": "Unicode attribution", "provenance": strings.Repeat("界", 500)})
	if unicode["status"] != "inserted" {
		t.Fatalf("character count became byte count: %v", unicode)
	}
}

func TestMemoryV1ImmediateSearchAndCacheFailure(t *testing.T) {
	h, _ := newTestHandlers(t)
	saved := acceptanceCall(t, h, "remember", map[string]any{"fact": "immediatecachemarker one", "provenance": "cache fixture"})
	got := acceptanceCall(t, h, "recall", map[string]any{"query": "immediatecachemarker"})
	if len(got["results"].([]any)) != 1 {
		t.Fatalf("durable source not immediately searchable: %v saved=%v", got, saved)
	}
	duplicate := acceptanceCall(t, h, "remember", map[string]any{"fact": "immediatecachemarker one", "provenance": "cache fixture"})
	if duplicate["status"] != "duplicate" || duplicate["id"] != saved["id"] {
		t.Fatalf("cache refresh duplicated fact: %v", duplicate)
	}
	chunks, err := h.deps.Index.AllChunks(t.Context())
	if err != nil || len(chunks) != 1 {
		t.Fatalf("duplicate refresh inflated chunks: %v %v", chunks, err)
	}
	if err := h.deps.Index.Close(); err != nil {
		t.Fatal(err)
	}
	degraded := acceptanceCall(t, h, "remember", map[string]any{"fact": "cacheclosedmarker two", "provenance": "cache fixture"})
	if degraded["status"] != "inserted" || !strings.Contains(degraded["status_text"].(string), "sync") {
		t.Fatalf("cache failure hid durable write: %v", degraded)
	}
	proj, err := store.LoadMemoryProjection(h.deps.Sources)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := proj.Get(degraded["id"].(string)); !ok {
		t.Fatal("reported insert not durable")
	}
}

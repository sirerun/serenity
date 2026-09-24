package memory

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sirerun/serenity/internal/router"
)

func mustRemember(t *testing.T, h *Handlers, ctx context.Context, req rememberRequest) rememberResponse {
	t.Helper()
	v, bad, err := h.remember(ctx, mustMarshal(t, req))
	if err != nil || bad {
		t.Fatalf("remember: %v %+v", err, v)
	}
	return v.(rememberResponse)
}

func readMemoryFactByID(t *testing.T, h *Handlers, ctx context.Context, id string) (readMemoryFactResponse, VerbError, bool) {
	t.Helper()
	v, bad, err := h.readMemoryFact(ctx, mustMarshal(t, map[string]string{"id": id}))
	if err != nil {
		t.Fatalf("read_memory_fact: unexpected internal error: %v", err)
	}
	if bad {
		return readMemoryFactResponse{}, v.(VerbError), true
	}
	return v.(readMemoryFactResponse), VerbError{}, false
}

// TestReadMemoryFactExactID is this task's "exact vs fuzzy ids" acc line,
// positive half: the exact opaque fact_id resolves the live fact, with no
// search/entity fallback involved.
func TestReadMemoryFactExactID(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	rem := mustRemember(t, h, ctx, rememberRequest{
		Fact: "exact read target", Provenance: "test", Entity: "people/exact-read-target",
	})

	resp, verbErr, bad := readMemoryFactByID(t, h, ctx, rem.ID)
	if bad {
		t.Fatalf("read by opaque id failed: %+v", verbErr)
	}
	if resp.ID != rem.ID || resp.Fact != "exact read target" || resp.Provenance != "test" ||
		resp.Kind != "fact" || resp.Visibility != "world" || !resp.ContentUntrusted {
		t.Fatalf("unexpected response for opaque id: %+v", resp)
	}
	if resp.EntitySlug == nil || *resp.EntitySlug != "exact-read-target" {
		t.Fatalf("entity_slug not echoed: %+v", resp)
	}
	if resp.ValidUntil != nil {
		t.Fatalf("valid_until should be nil for a never-expiring fact: %+v", resp)
	}
}

// TestReadMemoryFactRejectsLegacyID is root review point 1's own acc line:
// this brand-new tool carries no compatibility obligation to accept the
// pre-v1 legacy decimal id forget/recall still support, so even a REAL
// legacy id for a REAL live fact (obtained here the same way a caller
// would, via recall) must still be rejected as invalid_params -- proving
// the restriction is not merely "no legacy-shaped string happens to
// exist" but an actual, enforced scope narrowing.
func TestReadMemoryFactRejectsLegacyID(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()
	rem := mustRemember(t, h, ctx, rememberRequest{Fact: "legacy rejection probe", Provenance: "test", Entity: "people/legacy-rejection"})

	recallResp, bad, err := h.recall(ctx, mustMarshal(t, recallRequest{Entity: "people/legacy-rejection"}))
	if err != nil || bad {
		t.Fatalf("recall: %v %+v", err, recallResp)
	}
	facts := recallResp.(recallResponse).Facts
	if len(facts) != 1 || facts[0].FactID != rem.ID {
		t.Fatalf("recall did not return the remembered fact: %+v", facts)
	}
	legacyID := strconv.FormatInt(facts[0].ID, 10)

	_, verbErr, bad2 := readMemoryFactByID(t, h, ctx, legacyID)
	if !bad2 {
		t.Fatalf("legacy id %q for a real live fact: want rejected, got a live response", legacyID)
	}
	if verbErr.Error != ErrCodeInvalidParams {
		t.Fatalf("legacy id %q: error = %q, want %q", legacyID, verbErr.Error, ErrCodeInvalidParams)
	}
}

// TestReadMemoryFactRejectsInvalidShapes is this task's "invalid ids" acc
// line: anything that is not the exact 64-lowercase-hex opaque fact_id
// shape is rejected with invalid_params before any lookup runs.
func TestReadMemoryFactRejectsInvalidShapes(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()
	rem := mustRemember(t, h, ctx, rememberRequest{Fact: "shape probe", Provenance: "test"})

	cases := map[string]string{
		"empty":          "",
		"page_slug":      "people/alice",
		"search_query":   "onboarding decision",
		"truncated_sha":  rem.ID[:32],
		"uppercase_sha":  strings.ToUpper(rem.ID),
		"non_hex_chars":  "zz" + rem.ID[2:],
		"legacy_decimal": "1",
		"path_traversal": "../../etc/passwd",
	}
	for name, id := range cases {
		t.Run(name, func(t *testing.T) {
			_, verbErr, bad := readMemoryFactByID(t, h, ctx, id)
			if !bad {
				t.Fatalf("id %q: want rejected, got a live response", id)
			}
			if verbErr.Error != ErrCodeInvalidParams {
				t.Fatalf("id %q: error = %q, want %q", id, verbErr.Error, ErrCodeInvalidParams)
			}
		})
	}
}

// TestReadMemoryFactBoundedResponse is root review's own acc line, now
// corrected twice: remember has no fact-text length limit (only provenance
// is bounded), so an eligible fact can still serialize larger than this
// tool's own bound. Root's second interim observation named a specific
// prior bug: measuring only json.Marshal(resp) (the inner domain object)
// under-counts the real bound, because textResult then embeds that JSON as
// a Go string inside mcp.Result.Content[0].Text, and every quote/backslash
// in it gets escaped a SECOND time when the outer envelope is marshaled.
// This proves the fix by picking a fact built from a quote/backslash/
// control-character/multibyte unit, sized so the (wrong) inner-only size
// still fits the bound -- reproducing the exact prior bug's blind spot --
// while the REAL mcp.Result envelope, produced by calling the actual
// registered tool handler (not the h.readMemoryFact bypass every other
// case in this file uses), does not.
func TestReadMemoryFactBoundedResponse(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	// A quote and a backslash each double their already-escaped length a
	// second time on re-embedding (`\"` -> `\\\"`); a control character
	// (newline) and a multibyte rune (CJK, 3 UTF-8 bytes) do not grow the
	// same way, so mixing all four isolates the double-escaping gap
	// specifically instead of just being "big."
	const unit = "\"\\\n世"
	const repeats = 100000
	fact := strings.Repeat(unit, repeats)
	rem := mustRemember(t, h, ctx, rememberRequest{Fact: fact, Provenance: "test"})

	// The inner-only measurement root's review found insufficient: the
	// exact domain response a live read produces, marshaled alone. This
	// must still report "fits" -- proving the scenario actually reproduces
	// the prior bug's blind spot, not a straw man.
	inner := readMemoryFactResponse{
		ProtocolVersion:  ProtocolVersion,
		ID:               rem.ID,
		Fact:             fact,
		Kind:             "fact",
		Visibility:       "world",
		Provenance:       "test",
		ContentUntrusted: true,
	}
	innerBytes, err := json.Marshal(inner)
	if err != nil {
		t.Fatalf("marshal expected inner response: %v", err)
	}
	if len(innerBytes) > readMemoryFactMaxResponseBytes {
		t.Fatalf("test setup: inner-only size %d already exceeds the bound %d -- this no longer isolates the double-escaping gap", len(innerBytes), readMemoryFactMaxResponseBytes)
	}

	// The real call, through the actual registered mcp.Tool.Handler --
	// the same Content[0].Text construction production uses, not a
	// bypass of it.
	tool := h.readMemoryFactTool()
	result, err := tool.Handler(ctx, mustMarshal(t, map[string]string{"id": rem.ID}))
	if err != nil {
		t.Fatalf("read_memory_fact handler: unexpected transport error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("want the real envelope refused (inner-only was %d bytes, fits in the %d-byte bound), got a live response", len(innerBytes), readMemoryFactMaxResponseBytes)
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatalf("unexpected result shape: %+v", result)
	}

	var verbErr VerbError
	if err := json.Unmarshal([]byte(result.Content[0].Text), &verbErr); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if verbErr.Error != ErrCodeUnavailable {
		t.Fatalf("error = %q, want %q (reused, not a new core error code)", verbErr.Error, ErrCodeUnavailable)
	}
	if strings.Contains(verbErr.Message, unit) || strings.Contains(verbErr.Suggestion, unit) {
		t.Fatal("refusal must not echo the oversize content itself")
	}
	lower := strings.ToLower(verbErr.Message + " " + verbErr.Suggestion)
	if strings.Contains(lower, "bulk") || strings.Contains(lower, "import") {
		t.Fatal("refusal must not claim an unsupported bulk/import read fallback")
	}
}

// TestReadMemoryFactUniformUnavailable is this task's own core acceptance
// line: a well-formed id that is missing, private, TTL-independent-but-
// forgotten, or operation-key-canceled must ALL return the identical
// unavailable envelope -- never not_found or scope_denied -- so a caller
// cannot learn from the failure shape whether a private/withdrawn fact
// exists under a given id (RFC-EXACT-READ-01's own stated constraint).
func TestReadMemoryFactUniformUnavailable(t *testing.T) {
	h, _ := newTestHandlers(t)
	ctx := context.Background()

	// A syntactically valid but never-written sha.
	missingID := strings.Repeat("0", 64)

	private := mustRemember(t, h, ctx, rememberRequest{Fact: "private probe", Provenance: "test", Visibility: "private"})

	forgotten := mustRemember(t, h, ctx, rememberRequest{Fact: "forget probe", Provenance: "test"})
	if _, bad, err := h.forget(ctx, mustMarshal(t, map[string]string{"id": forgotten.ID})); err != nil || bad {
		t.Fatalf("forget: %v", err)
	}

	keyed := mustRemember(t, h, ctx, rememberRequest{OperationKey: "exact-read-cancel-1", Fact: "cancel probe", Provenance: "test"})
	// Read BEFORE cancellation must succeed -- proves the writer's fresh
	// state is visible, not just its absence.
	if _, verbErr, bad := readMemoryFactByID(t, h, ctx, keyed.ID); bad {
		t.Fatalf("read before cancellation unexpectedly failed: %+v", verbErr)
	}
	if _, bad, err := h.cancelOperation(ctx, mustMarshal(t, map[string]string{"operation_key": "exact-read-cancel-1"})); err != nil || bad {
		t.Fatalf("cancel_memory_operation: %v", err)
	}

	cases := map[string]string{
		"missing":   missingID,
		"private":   private.ID,
		"forgotten": forgotten.ID,
		"canceled":  keyed.ID, // read AFTER cancellation must now fail.
	}

	var first VerbError
	for name, id := range cases {
		t.Run(name, func(t *testing.T) {
			_, verbErr, bad := readMemoryFactByID(t, h, ctx, id)
			if !bad {
				t.Fatalf("id %q (%s): want unavailable, got a live response", id, name)
			}
			if verbErr.Error != ErrCodeUnavailable {
				t.Fatalf("%s: error = %q, want %q", name, verbErr.Error, ErrCodeUnavailable)
			}
			if first.Error == "" {
				first = verbErr
				return
			}
			if verbErr.Message != first.Message || verbErr.Suggestion != first.Suggestion {
				t.Fatalf("%s: envelope diverges from the first case -- missing/private/expired/canceled must be indistinguishable\ngot:  %+v\nwant: %+v", name, verbErr, first)
			}
		})
	}
}

// TestReadMemoryFactNoProviderCall proves read_memory_fact never reaches
// the embedder or composer -- a poisoned double fails the test the moment
// either is invoked, so a silent regression toward a search/synthesis path
// is caught, not just documented.
func TestReadMemoryFactNoProviderCall(t *testing.T) {
	root := gitRepoFixture(t)
	deps, closeFn := testDeps(t, root)
	t.Cleanup(closeFn)

	var embedCalls, composeCalls atomic.Int32
	deps.Embedder = poisonEmbedder{calls: &embedCalls}
	deps.Composer = poisonComposer{calls: &composeCalls}
	h := New(deps)
	ctx := context.Background()

	rem := mustRemember(t, h, ctx, rememberRequest{Fact: "no provider probe", Provenance: "test"})
	// remember's OWN search-indexing pipeline legitimately calls the
	// embedder (index.RefreshMemoryFactSearch) when an index is configured
	// -- that is unrelated to read_memory_fact and out of this test's
	// scope. Baseline after remember, then assert the READ itself adds no
	// further calls, rather than asserting zero calls across the whole
	// test (which would also fail on remember's own unrelated behavior).
	embedBefore, composeBefore := embedCalls.Load(), composeCalls.Load()
	if _, verbErr, bad := readMemoryFactByID(t, h, ctx, rem.ID); bad {
		t.Fatalf("read: %+v", verbErr)
	}
	if n := embedCalls.Load() - embedBefore; n != 0 {
		t.Fatalf("read_memory_fact called the embedder %d times, want 0", n)
	}
	if n := composeCalls.Load() - composeBefore; n != 0 {
		t.Fatalf("read_memory_fact called the composer %d times, want 0", n)
	}
}

type poisonEmbedder struct{ calls *atomic.Int32 }

func (poisonEmbedder) ModelVersion() string { return "poison@v1" }
func (p poisonEmbedder) Embed(context.Context, string) ([]float32, error) {
	p.calls.Add(1)
	return nil, nil
}

type poisonComposer struct{ calls *atomic.Int32 }

func (p poisonComposer) Complete(context.Context, router.TaskClass, router.Prompt, router.Budget) (router.Result, error) {
	p.calls.Add(1)
	return router.Result{}, nil
}
